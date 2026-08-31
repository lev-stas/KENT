package kubernetes

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestMapCoreEventCarriesActionAndReportingInstance(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())

	tests := []struct {
		name                  string
		reportingInstance     string
		sourceHost            string
		action                string
		wantAction            string
		wantReportingInstance string
	}{
		{
			name:                  "modern recorder fills both fields",
			reportingInstance:     "kubelet-node-1",
			sourceHost:            "node-1",
			action:                "Binding",
			wantAction:            "Binding",
			wantReportingInstance: "kubelet-node-1",
		},
		{
			name:                  "legacy recorder falls back to the source host",
			sourceHost:            "node-1",
			wantReportingInstance: "node-1",
		},
		{
			name: "neither reported leaves the fields empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ev, err := mapK8sEventToDomain(&corev1.Event{
				ObjectMeta:        metav1.ObjectMeta{Name: "pod-1.abc", Namespace: "default", UID: types.UID("uid-1")},
				InvolvedObject:    corev1.ObjectReference{Kind: "Pod", Name: "pod-1", Namespace: "default"},
				Reason:            "Started",
				Message:           "container started",
				Type:              "Normal",
				Source:            corev1.EventSource{Component: "kubelet", Host: tc.sourceHost},
				Action:            tc.action,
				ReportingInstance: tc.reportingInstance,
				FirstTimestamp:    now,
				LastTimestamp:     now,
				Count:             1,
			})
			if err != nil {
				t.Fatalf("mapK8sEventToDomain returned error: %v", err)
			}

			if ev.Action() != tc.wantAction {
				t.Fatalf("Action() = %q, want %q", ev.Action(), tc.wantAction)
			}
			if ev.ReportingInstance() != tc.wantReportingInstance {
				t.Fatalf("ReportingInstance() = %q, want %q", ev.ReportingInstance(), tc.wantReportingInstance)
			}
			if ev.Object().Name != "pod-1" {
				t.Fatalf("involved object name = %q, want pod-1", ev.Object().Name)
			}
		})
	}
}

// An event recorded through events.k8s.io/v1 and read back through core/v1
// names its reporter in ReportingController, not in Source.
func TestMapCoreEventFallsBackToReportingController(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())

	tests := []struct {
		name                string
		sourceComponent     string
		reportingController string
		want                string
	}{
		{name: "legacy source wins", sourceComponent: "kubelet", reportingController: "kubelet-controller", want: "kubelet"},
		{name: "empty source falls back", reportingController: "deployment-controller", want: "deployment-controller"},
		{name: "neither reported stays empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ev, err := mapK8sEventToDomain(&corev1.Event{
				ObjectMeta:          metav1.ObjectMeta{Name: "pod-1.abc", Namespace: "default", UID: types.UID("uid-1")},
				InvolvedObject:      corev1.ObjectReference{Kind: "Pod", Name: "pod-1", Namespace: "default"},
				Reason:              "Started",
				Message:             "container started",
				Type:                "Normal",
				Source:              corev1.EventSource{Component: tc.sourceComponent},
				ReportingController: tc.reportingController,
				FirstTimestamp:      now,
				Count:               1,
			})
			if err != nil {
				t.Fatalf("mapK8sEventToDomain returned error: %v", err)
			}

			if ev.Source() != tc.want {
				t.Fatalf("Source() = %q, want %q", ev.Source(), tc.want)
			}
		})
	}
}

func TestMapEventV1CarriesActionAndReportingInstance(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Now())

	tests := []struct {
		name                  string
		reportingInstance     string
		deprecatedHost        string
		action                string
		wantAction            string
		wantReportingInstance string
	}{
		{
			name:                  "events/v1 recorder fills both fields",
			reportingInstance:     "kubelet-node-1",
			action:                "Binding",
			wantAction:            "Binding",
			wantReportingInstance: "kubelet-node-1",
		},
		{
			name:                  "mirrored core/v1 event falls back to the deprecated source host",
			deprecatedHost:        "node-1",
			wantReportingInstance: "node-1",
		},
		{
			name: "neither reported leaves the fields empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ev, err := mapK8sEventV1ToDomain(&eventv1.Event{
				ObjectMeta:          metav1.ObjectMeta{Name: "pod-1.abc", Namespace: "default", UID: types.UID("uid-1")},
				Regarding:           corev1.ObjectReference{Kind: "Pod", Name: "pod-1", Namespace: "default"},
				Reason:              "Started",
				Note:                "container started",
				Type:                "Normal",
				ReportingController: "kubelet",
				ReportingInstance:   tc.reportingInstance,
				Action:              tc.action,
				DeprecatedSource:    corev1.EventSource{Component: "kubelet", Host: tc.deprecatedHost},
				EventTime:           metav1.MicroTime{Time: now.Time},
			})
			if err != nil {
				t.Fatalf("mapK8sEventV1ToDomain returned error: %v", err)
			}

			if ev.Action() != tc.wantAction {
				t.Fatalf("Action() = %q, want %q", ev.Action(), tc.wantAction)
			}
			if ev.ReportingInstance() != tc.wantReportingInstance {
				t.Fatalf("ReportingInstance() = %q, want %q", ev.ReportingInstance(), tc.wantReportingInstance)
			}
			if ev.Object().Name != "pod-1" {
				t.Fatalf("involved object name = %q, want pod-1", ev.Object().Name)
			}
		})
	}
}
