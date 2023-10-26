package machine

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MachineController", func() {

	Context("When Evaluating a deleting machine for blocking customer workloads", func() {
		Context("When parsing the list of events firing on a cluster", func() {
			Context("returns nil when no 'DrainRequeued' events are found", func() {
				It("handles an empty event list", func() {
					eventList := &corev1.EventList{
						Items: []corev1.Event{},
					}
					event, err := getMostRecentDrainFailedEvent(eventList)
					Expect(err).Should(MatchError(errNoEvents))
					Expect(event).To(BeNil())
				})
				It("handles an event list with no DrainRequeued events", func() {
					eventList := &corev1.EventList{
						Items: []corev1.Event{
							{Reason: "DrainProceeds"},
							{Reason: "SomeOtherReason"},
						},
					}

					event, err := getMostRecentDrainFailedEvent(eventList)
					Expect(err).To(BeNil())
					Expect(event).To(BeNil())
				})
			})

			It("Returns the newest event if no matter where in the list it is", func() {
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

				Expect(err1).To(BeNil())
				Expect(err2).To(BeNil())
				Expect(err3).To(BeNil())

				Expect(newestEventFirst).NotTo(BeNil())
				Expect(newestEventMiddle).NotTo(BeNil())
				Expect(newestEventLast).NotTo(BeNil())

				Expect(newestEventFirst.Message).To(ContainSubstring("Newest Drain Event"))
				Expect(newestEventMiddle.Message).To(ContainSubstring("Newest Drain Event"))
				Expect(newestEventLast.Message).To(ContainSubstring("Newest Drain Event"))
			})
		})
		Context("When parsing the pod names and namespaces from an event", func() {
			It("should return an empty map if there are no matches", func() {
				event := &corev1.Event{Message: "Should not match"}
				pods := parsePodsAndNamespacesFromEvent(event)

				Expect(pods).To(BeEmpty())
			})
			It("should return the correct amount of matches for a single pod", func() {
				event := &corev1.Event{Message: "pods/\"customer-pod\" -n \"test\" failed to drain"}
				pods := parsePodsAndNamespacesFromEvent(event)

				Expect(pods).To(HaveLen(1))
			})
			It("Should return the correct amount of matches if an openshift-pod is present", func() {
				event := &corev1.Event{Message: "pods/\"osd-pod\" -n \"openshift-namespace\" does not exist; pods/\"customer-pod\" -n \"test\" failed to drain"}
				pods := parsePodsAndNamespacesFromEvent(event)

				Expect(pods).To(HaveLen(1))
			})
			It("Should return the correct amount of matches for multiple pods", func() {
				event := &corev1.Event{Message: "pods/\"foo\" -n \"bar\" does not exist; pods/\"baz\" -n \"bat\" failed to drain"}
				pods := parsePodsAndNamespacesFromEvent(event)

				Expect(pods).To(HaveLen(2))
				Expect(pods["foo"]).To(Equal("bar"))
				Expect(pods["baz"]).To(Equal("bat"))
			})
		})
	})
})
