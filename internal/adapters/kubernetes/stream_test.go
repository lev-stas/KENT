package kubernetes

import (
	"context"
	"sync"
	"testing"
	"time"

	"event_exporter/internal/domain"
	"event_exporter/internal/pkg/logger"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

func testK8sEvent(uid, ns, rv string) *corev1.Event {
	now := metav1.NewTime(time.Now())
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "event-" + uid,
			Namespace:       ns,
			UID:             types.UID(uid),
			ResourceVersion: rv,
		},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "pod-1", Namespace: ns},
		Reason:         "Started",
		Message:        "container started",
		Type:           "Normal",
		Source:         corev1.EventSource{Component: "kubelet"},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
}

// fakeWatchProvider hands out pre-built fake watchers and records the
// ListOptions of every startWatch call.
type fakeWatchProvider struct {
	mu       sync.Mutex
	calls    []metav1.ListOptions
	watchers []*watch.FakeWatcher
	started  chan int
}

func newFakeWatchProvider(count int) *fakeWatchProvider {
	p := &fakeWatchProvider{started: make(chan int, count+1)}
	for i := 0; i < count; i++ {
		p.watchers = append(p.watchers, watch.NewFakeWithChanSize(10, false))
	}
	return p
}

func (p *fakeWatchProvider) start(_ context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	p.mu.Lock()
	idx := len(p.calls)
	p.calls = append(p.calls, opts)
	var w *watch.FakeWatcher
	if idx < len(p.watchers) {
		w = p.watchers[idx]
	} else {
		w = watch.NewFakeWithChanSize(10, false)
	}
	p.mu.Unlock()
	p.started <- idx
	return w, nil
}

func (p *fakeWatchProvider) call(i int) metav1.ListOptions {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[i]
}

func waitStart(t *testing.T, p *fakeWatchProvider, want int) {
	t.Helper()
	select {
	case idx := <-p.started:
		if idx != want {
			t.Fatalf("expected watch start #%d, got #%d", want, idx)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for watch start #%d", want)
	}
}

func newTestSession(p *fakeWatchProvider) *watchSession {
	return &watchSession{
		logger: logger.New("error"),
		scope:  "test",
		startWatch: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			return p.start(ctx, opts)
		},
		mapEvent: func(obj runtime.Object) (*domain.Event, error) {
			e, ok := obj.(*corev1.Event)
			if !ok {
				return nil, nil
			}
			return mapK8sEventToDomain(e)
		},
		initialBackoff: time.Millisecond,
		maxBackoff:     5 * time.Millisecond,
	}
}

func receiveEvent(t *testing.T, out <-chan *domain.Event) *domain.Event {
	t.Helper()
	select {
	case ev := <-out:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
		return nil
	}
}

func TestWatchSessionResumesFromLastResourceVersion(t *testing.T) {
	t.Parallel()

	p := newFakeWatchProvider(2)
	session := newTestSession(p)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *domain.Event, 10)
	done := make(chan struct{})
	go func() {
		_ = session.run(ctx, out)
		close(done)
	}()

	waitStart(t, p, 0)
	if got := p.call(0).ResourceVersion; got != "" {
		t.Fatalf("first watch should start without resourceVersion, got %q", got)
	}
	if !p.call(0).AllowWatchBookmarks {
		t.Fatal("watch must request bookmarks")
	}

	p.watchers[0].Add(testK8sEvent("uid-1", "default", "100"))
	if ev := receiveEvent(t, out); ev.UID() != "uid-1" {
		t.Fatalf("unexpected event uid: %s", ev.UID())
	}

	// Bookmark carries a newer resourceVersion but must not produce an event.
	p.watchers[0].Action(watch.Bookmark, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{ResourceVersion: "200"},
	})
	// Deletion of an aged-out Event object must not be re-exported either.
	p.watchers[0].Delete(testK8sEvent("uid-2", "default", "300"))

	p.watchers[0].Stop()

	waitStart(t, p, 1)
	if got := p.call(1).ResourceVersion; got != "300" {
		t.Fatalf("expected reconnect to resume from resourceVersion 300, got %q", got)
	}

	select {
	case ev := <-out:
		t.Fatalf("bookmark/deleted must not produce events, got uid %s", ev.UID())
	default:
	}

	cancel()
	<-done
}

func TestWatchSessionResetsResourceVersionOnExpired(t *testing.T) {
	t.Parallel()

	p := newFakeWatchProvider(3)
	session := newTestSession(p)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *domain.Event, 10)
	done := make(chan struct{})
	go func() {
		_ = session.run(ctx, out)
		close(done)
	}()

	waitStart(t, p, 0)
	p.watchers[0].Add(testK8sEvent("uid-1", "default", "100"))
	receiveEvent(t, out)
	p.watchers[0].Stop()

	waitStart(t, p, 1)
	if got := p.call(1).ResourceVersion; got != "100" {
		t.Fatalf("expected resume from 100, got %q", got)
	}

	p.watchers[1].Error(&metav1.Status{
		Code:   410,
		Reason: metav1.StatusReasonExpired,
	})
	p.watchers[1].Stop()

	waitStart(t, p, 2)
	if got := p.call(2).ResourceVersion; got != "" {
		t.Fatalf("expected fresh watch after 410 Gone, got resourceVersion %q", got)
	}

	cancel()
	<-done
}

func TestWatchSessionFiltersNamespaces(t *testing.T) {
	t.Parallel()

	p := newFakeWatchProvider(1)
	session := newTestSession(p)
	session.excludeNS = toSet([]string{"kube-system"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := make(chan *domain.Event, 10)
	done := make(chan struct{})
	go func() {
		_ = session.run(ctx, out)
		close(done)
	}()

	waitStart(t, p, 0)
	p.watchers[0].Add(testK8sEvent("uid-sys", "kube-system", "100"))
	p.watchers[0].Add(testK8sEvent("uid-app", "default", "101"))

	if ev := receiveEvent(t, out); ev.UID() != "uid-app" {
		t.Fatalf("expected kube-system event to be filtered, got uid %s", ev.UID())
	}

	cancel()
	<-done
}
