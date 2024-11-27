package config

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	resources := &DGateResources{
		SkipValidation: false,
		Namespaces: []spec.Namespace{
			{
				Name: "default",
				Tags: []string{"default"},
			},
		},
		Services: []spec.Service{
			{
				Name:          "default",
				NamespaceName: "default",
				Tags:          []string{"default"},
			},
		},
		Routes: []spec.Route{
			{
				Name:          "default",
				Tags:          []string{"default"},
				NamespaceName: "default",
			},
		},
		Domains: []DomainSpec{
			{
				Domain: spec.Domain{
					Name:          "default",
					NamespaceName: "default",
					Tags:          []string{"default"},
				},
			},
		},
		Modules: []ModuleSpec{
			{
				Module: spec.Module{
					Name:          "default",
					NamespaceName: "default",
					Payload:       "void(0)",
					Tags:          []string{"default"},
				},
			},
		},
	}
	changes, err := resources.Validate()
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, 5, changes)
}
