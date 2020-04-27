package dronet_operator

import (
	"fmt"
	"github.com/netgroup-polito/dronev2/internal/errdefs"
	"golang.org/x/tools/go/ssa/interp/testdata/src/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"net"
	"os"
	"strings"
)

func getPodIP() (net.IP, error) {
	ipAddress, isSet := os.LookupEnv("POD_IP")
	if isSet == false {
		return nil, errdefs.NotFound("the pod IP is not set")
	}
	if ipAddress == "" {
		return nil, errors.New("pod IP is not yet set")
	}
	return net.ParseIP(ipAddress), nil
}

func GetNodeName() (string, error){
	nodeName, isSet := os.LookupEnv("NODE_NAME")
	if isSet == false {
		return nodeName, errdefs.NotFound("NODE_NAME has not been set. check you manifest file")
	}
	return nodeName, nil
}

func GetClusterPodCIDR() (string, error){
	podCIDR, isSet := os.LookupEnv("POD_CIDR")
	if isSet == false {
		return podCIDR, errdefs.NotFound("POD_CIDR has not been set. check you manifest file")
	}
	return podCIDR, nil
}

func getInternalIPOfNode(node corev1.Node) (string, error) {
	var internalIp string
	for _, address := range node.Status.Addresses {
		if address.Type == "InternalIP" {
			internalIp = address.Address
			break
		}
	}
	if internalIp == "" {
		klog.V(4).Infof("internalIP of the node not found, probably is not set")
		return internalIp, errdefs.NotFound("internalIP of the node is not set")
	}
	return internalIp, nil
}

func getOverlayCIDR() (*net.IPNet, error) {
	//VXLAN_CIDR has to be in the following format: xxx.xxx.xxx.xxx/yy
	vxlanCidr, isSet := os.LookupEnv("VXLAN_CIDR")
	if isSet == false {
		return nil, errdefs.NotFound("VXLAN_CIDR is not set")
	}
	_, vxlanNet, err := net.ParseCIDR(vxlanCidr)
	if err != nil {
		return nil, fmt.Errorf("unable to convert the VXLAN_CIDR in *IPNet format: %v", err)
	}
	return vxlanNet, nil
}

func IsGatewayNode(clientset *kubernetes.Clientset) (bool, error) {
	isGatewayNode := false
	//retrieve the node which is labeled as the gateway
	nodesList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector: "dronet.drone.com/gateway == true"})
	if err != nil {
		logger.Error(err, "Unable to list nodes with labbel 'dronet.drone.com/gateway=true'")
		return isGatewayNode, fmt.Errorf("Unable to list nodes with labbel 'dronet.drone.com/gateway=true': %v", err)
	}
	if len(nodesList.Items) != 1 {
		klog.V(4).Infof("number of gateway nodes found: %d", len(nodesList.Items))
		return isGatewayNode, errdefs.NotFound("no gateway node has been found")
	}
	//check if my ip node is the same as the internal ip of the gateway node
	podIP, err := getPodIP()
	if err != nil {
		return isGatewayNode, err
	}
	internalIP, err := getInternalIPOfNode(nodesList.Items[0])
	if err != nil {
		return isGatewayNode, fmt.Errorf("unable to get internal ip of the gateway node: %v", err)
	}
	if podIP.String() == internalIP {
		isGatewayNode = true
		return isGatewayNode, nil
	} else {
		return isGatewayNode, nil
	}
}
func GetGatewayVxlanIP (clientset *kubernetes.Clientset) (string, error){
	var gatewayVxlanIP string
	//retrieve the node which is labeled as the gateway
	nodesList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector: "dronet.drone.com/gateway == true"})
	if err != nil {
		logger.Error(err, "Unable to list nodes with labbel 'dronet.drone.com/gateway=true'")
		return gatewayVxlanIP, fmt.Errorf("Unable to list nodes with labbel 'dronet.drone.com/gateway=true': %v", err)
	}
	if len(nodesList.Items) != 1 {
		klog.V(4).Infof("number of gateway nodes found: %d", len(nodesList.Items))
		return gatewayVxlanIP, errdefs.NotFound("no gateway node has been found")
	}
	internalIP, err := getInternalIPOfNode(nodesList.Items[0])
	if err != nil {
		return gatewayVxlanIP, fmt.Errorf("unable to get internal ip of the gateway node: %v", err)
	}
	vxlanCIDR := "192.168.200.0"
	//derive IP for the vxlan device
	//take the last octet of the podIP
	//TODO: use & and | operators with masks
	temp := strings.Split(internalIP, ".")
	temp1 := strings.Split(vxlanCIDR, ".")
	gatewayVxlanIP = temp1[0] + "." + temp1[1] + "." + temp1[2] + "." + temp[3]
	return gatewayVxlanIP, nil
}
func getRemoteVTEPS(clientset *kubernetes.Clientset) ([]string, error) {
	var remoteVTEP []string
	nodesList, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{LabelSelector: "type != virtual-node"})
	if err != nil {
		logger.Error(err, "Unable to list nodes with labbel 'dronet.drone.com/gateway=true'")
		return nil, fmt.Errorf("Unable to list nodes with labbel 'type != virtual-node': %v", err)
	}
	//get my podIP so i don't put consider it a s VTEP
	podIP, err := getPodIP()
	if err != nil {
		return nil, fmt.Errorf("unable to get pod ip while getting remoteVTEPs: %v", err)
	}
	//populate the VTEPs
	for _, node := range nodesList.Items {
		internalIP, err := getInternalIPOfNode(node)
		if err != nil {
			//log the error but don't exit
			logger.Error(err, "unable to get internal ip of the node named -> %s", node.Name)
		}
		if internalIP != podIP.String() {
			remoteVTEP = append(remoteVTEP, internalIP)
		}
	}
	return remoteVTEP, nil
}

// Helper functions to check if a string is contained in a slice of strings.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
// Helper functions to check and remove string from a slice of strings.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}