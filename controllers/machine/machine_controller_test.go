package machine

import (
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = ginkgo.Describe("MachineController", func() {

	ginkgo.Context("When Evaluating a deleting machine for blocking customer workloads", func() {
		ginkgo.Context("When parsing the list of events firing on a cluster", func() {
			ginkgo.Context("returns nil when no 'DrainRequeued' events are found", func() {
				ginkgo.It("handles an empty event list", func() {
					eventList := &corev1.EventList{
						Items: []corev1.Event{},
					}
					event, err := getMostRecentDrainFailedEvent(eventList)
					gomega.Expect(err).Should(gomega.MatchError(errNoEvents))
					gomega.Expect(event).To(gomega.BeNil())
				})
				ginkgo.It("handles an event list with no DrainRequeued events", func() {
					eventList := &corev1.EventList{
						Items: []corev1.Event{
							{Reason: "DrainProceeds"},
							{Reason: "SomeOtherReason"},
						},
					}

					event, err := getMostRecentDrainFailedEvent(eventList)
					gomega.Expect(err).To(gomega.BeNil())
					gomega.Expect(event).To(gomega.BeNil())
				})
			})

			ginkgo.It("Returns the newest event if no matter where in the list it is", func() {
				newestTime := metav1.Now()
				newerTime := metav1.NewTime(newestTime.Add(-5 * time.Minute))
				oldestTime := metav1.NewTime(newerTime.Add(-5 * time.Minute))
				newestEvent := corev1.Event{Reason: "DrainRequeued", LastTimestamp: newestTime, Message: "error when evicting pods: Newest Drain Event"}
				newerEvent := corev1.Event{Reason: "DrainRequeued", LastTimestamp: newerTime, Message: "error when evicting pods: This is a newer drain event"}
				pastEvent := corev1.Event{Reason: "DrainRequeued", LastTimestamp: oldestTime, Message: "error when evicting pods: This is the oldest drain event"}

				newestFirst := &corev1.EventList{
					Items: []corev1.Event{newestEvent, newerEvent, pastEvent},
				}
				newestMiddle := &corev1.EventList{
					Items: []corev1.Event{newerEvent, newestEvent, pastEvent},
				}
				newestLast := &corev1.EventList{
					Items: []corev1.Event{pastEvent, newerEvent, newestEvent},
				}

				newestEventFirst, err1 := getMostRecentDrainFailedEvent(newestFirst)
				newestEventMiddle, err2 := getMostRecentDrainFailedEvent(newestMiddle)
				newestEventLast, err3 := getMostRecentDrainFailedEvent(newestLast)

				gomega.Expect(err1).To(gomega.BeNil())
				gomega.Expect(err2).To(gomega.BeNil())
				gomega.Expect(err3).To(gomega.BeNil())

				gomega.Expect(newestEventFirst).NotTo(gomega.BeNil())
				gomega.Expect(newestEventMiddle).NotTo(gomega.BeNil())
				gomega.Expect(newestEventLast).NotTo(gomega.BeNil())

				gomega.Expect(newestEventFirst.Message).To(gomega.ContainSubstring("Newest Drain Event"))
				gomega.Expect(newestEventMiddle.Message).To(gomega.ContainSubstring("Newest Drain Event"))
				gomega.Expect(newestEventLast.Message).To(gomega.ContainSubstring("Newest Drain Event"))
			})
		})
		ginkgo.Context("When parsing the pod names and namespaces from an event", func() {
			ginkgo.It("should return an empty map if there are no matches", func() {
				event := &corev1.Event{Message: "Should not match"}
				pods := parsePodsAndNamespacesFromEvent(event)

				gomega.Expect(pods).To(gomega.BeEmpty())
			})
			ginkgo.It("should return the correct amount of matches for a single pod", func() {
				event := &corev1.Event{Message: "pods/\"customer-pod\" -n \"test\" failed to drain"}
				pods := parsePodsAndNamespacesFromEvent(event)

				gomega.Expect(pods).To(gomega.HaveLen(1))
			})
			ginkgo.It("Should return the correct amount of matches if an openshift-pod is present", func() {
				event := &corev1.Event{Message: "pods/\"osd-pod\" -n \"openshift-namespace\" does not exist; pods/\"customer-pod\" -n \"test\" failed to drain"}
				pods := parsePodsAndNamespacesFromEvent(event)

				gomega.Expect(pods).To(gomega.HaveLen(1))
			})
			ginkgo.It("Should return the correct amount of matches for multiple pods", func() {
				event := &corev1.Event{Message: "pods/\"foo\" -n \"bar\" does not exist; pods/\"baz\" -n \"bat\" failed to drain"}
				pods := parsePodsAndNamespacesFromEvent(event)

				gomega.Expect(pods).To(gomega.HaveLen(2))
				gomega.Expect(pods["foo"]).To(gomega.Equal("bar"))
				gomega.Expect(pods["baz"]).To(gomega.Equal("bat"))
			})
		})
	})
})
