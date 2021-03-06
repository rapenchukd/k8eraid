// Copyright 2019 Bloomberg Finance LP
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package queries

import (
	"fmt"
	"time"

	"github.com/bloomberg/k8eraid/pkgs/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PollNode function takes inputs and iterates across nodes in the kubernetes cluster, triggering alerts as needed.
func PollNode(
	clientset kubernetes.Interface,
	alertSpec types.NodeAlertSpec,
	tickertime int64,
	alertFn alertFunction,
	alertersConfig types.AlertersConfig,
) error {

	if alertSpec.ReportStatus.PendingThreshold == 0 {
		alertSpec.ReportStatus.PendingThreshold = 10
	}

	// Check rules with matching literal node name
	if alertSpec.Name != "*" {

		node, nodeerr := clientset.CoreV1().Nodes().Get(alertSpec.Name, metav1.GetOptions{})
		if nodeerr != nil {
			return &PollErr{
				Message: fmt.Sprintf("Unable to get node %s: %s", alertSpec.Name, nodeerr.Error()),
			}
		}

		checkNode(node, alertSpec, tickertime, alertFn, alertersConfig)

		// If nodename is a wildcard, list based on filter and iterate through
	} else {
		listopts := metav1.ListOptions{
			LabelSelector:        alertSpec.NodeFilter,
			IncludeUninitialized: false,
			Watch:                false,
			TimeoutSeconds:       &timeout,
		}

		// Check rules by label
		nodes, nodeserr := clientset.CoreV1().Nodes().List(listopts)
		if nodeserr != nil {
			return &PollErr{
				Message: fmt.Sprintf("Unable to get nodes: %s", nodeserr.Error()),
			}
		}

		// Check to see if there are the minimum specified nodes matching rule
		if int32(len(nodes.Items)) < alertSpec.ReportStatus.MinNodes {
			// ALERT
			alertmessage := fmt.Sprint("Node count with filter", alertSpec.NodeFilter, "in under minimum specification!")
			alertFn(alertSpec.AlerterType, alertSpec.AlerterName, alertmessage, alertersConfig)
		}

		// Iterate through node items
		for _, nodedata := range nodes.Items {
			node, nodeerr := clientset.CoreV1().Nodes().Get(nodedata.GetName(), metav1.GetOptions{})
			if nodeerr != nil {
				return &PollErr{
					Message: fmt.Sprintf("Unable to get node %s: %s", nodedata.Name, nodeerr.Error()),
				}
			}
			checkNode(node, alertSpec, tickertime, alertFn, alertersConfig)
		}
	}
	return nil
}

func checkNode(
	node *corev1.Node,
	alertSpec types.NodeAlertSpec,
	tickertime int64,
	alertFn alertFunction,
	alertersConfig types.AlertersConfig,
) {

	nowSeconds := time.Now().Unix()
	statusCreatedSecondsDiff := nowSeconds - node.ObjectMeta.CreationTimestamp.Unix()

	// If node hasnt been around longer than threshold, bail. otherwise check the status.
	if statusCreatedSecondsDiff > alertSpec.ReportStatus.PendingThreshold {
		for _, condition := range node.Status.Conditions {
			transitiontimeDiff := nowSeconds - condition.LastTransitionTime.Unix()
			if condition.Type == "Ready" {
				if transitiontimeDiff < tickertime && alertSpec.ReportStatus.NodeReady {
					// ALERT
					alertmessage := fmt.Sprint("Node", alertSpec.Name, "has changed ready status since last poll and may be restarting!")
					alertFn(alertSpec.AlerterType, alertSpec.AlerterName, alertmessage, alertersConfig)
					return
				}
			} else if condition.Type == "OutOfDisk" {
				if transitiontimeDiff < tickertime && alertSpec.ReportStatus.NodeOutOfDisk {
					// ALERT
					alertmessage := fmt.Sprint("Node", alertSpec.Name, "has changed OutOfDisk status since last poll and may have observed disk space issues!")
					alertFn(alertSpec.AlerterType, alertSpec.AlerterName, alertmessage, alertersConfig)
					return
				}
			} else if condition.Type == "MemoryPressure" {
				if transitiontimeDiff < tickertime && alertSpec.ReportStatus.NodeMemoryPressure {
					// ALERT
					alertmessage := fmt.Sprint("Node", alertSpec.Name, "has changed MemoryPressure status since last poll and may have observed memory pressure!")
					alertFn(alertSpec.AlerterType, alertSpec.AlerterName, alertmessage, alertersConfig)
					return
				}
			} else if condition.Type == "DiskPressure" {
				if transitiontimeDiff < tickertime && alertSpec.ReportStatus.NodeDiskPressure {
					// ALERT
					alertmessage := fmt.Sprint("Node", alertSpec.Name, "has changed DiskPressure tatus since last poll and may have observed disk pressure!")
					alertFn(alertSpec.AlerterType, alertSpec.AlerterName, alertmessage, alertersConfig)
					return
				}
			}
		}
	}
}
