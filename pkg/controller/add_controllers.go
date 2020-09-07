package controller

import (
	"github.com/openshift/osd-metrics-exporter/pkg/controller/clusterrole"
	"github.com/openshift/osd-metrics-exporter/pkg/controller/oauth"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, oauth.Add, clusterrole.Add)
}
