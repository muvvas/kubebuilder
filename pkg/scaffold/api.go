/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scaffold

import (
	"fmt"
	"path/filepath"
	"strings"

	"sigs.k8s.io/kubebuilder/internal/config"
	"sigs.k8s.io/kubebuilder/pkg/model"
	"sigs.k8s.io/kubebuilder/pkg/scaffold/input"
	"sigs.k8s.io/kubebuilder/pkg/scaffold/resource"
	"sigs.k8s.io/kubebuilder/pkg/scaffold/v1/controller"
	crdv1 "sigs.k8s.io/kubebuilder/pkg/scaffold/v1/crd"
	scaffoldv2 "sigs.k8s.io/kubebuilder/pkg/scaffold/v2"
	controllerv2 "sigs.k8s.io/kubebuilder/pkg/scaffold/v2/controller"
	crdv2 "sigs.k8s.io/kubebuilder/pkg/scaffold/v2/crd"
)

// API contains configuration for generating scaffolding for Go type
// representing the API and controller that implements the behavior for the API.
type API struct {
	// Plugins is the list of plugins we should allow to transform our generated scaffolding
	Plugins []Plugin

	Resource *resource.Resource

	config *config.Config

	// DoResource indicates whether to scaffold API Resource or not
	DoResource bool

	// DoController indicates whether to scaffold controller files or not
	DoController bool

	// Force indicates that the resource should be created even if it already exists.
	Force bool
}

// Validate validates whether API scaffold has correct bits to generate
// scaffolding for API.
func (api *API) Validate() error {
	if err := api.setDefaults(); err != nil {
		return err
	}
	if err := api.Resource.Validate(); err != nil {
		return err
	}

	if api.config.HasResource(api.Resource) && !api.Force {
		return fmt.Errorf("API resource already exists")
	}

	return nil
}

func (api *API) setDefaults() (err error) {
	if api.config == nil {
		api.config, err = config.Load()
		if err != nil {
			return
		}
	}

	return
}

func (api *API) Scaffold() error {
	if err := api.setDefaults(); err != nil {
		return err
	}

	switch {
	case api.config.IsV1():
		return api.scaffoldV1()
	case api.config.IsV2():
		return api.scaffoldV2()
	default:
		return fmt.Errorf("unknown project version %v", api.config.Version)
	}
}

func (api *API) buildUniverse(resource *resource.Resource) (*model.Universe, error) {
	return model.NewUniverse(
		model.WithConfig(&api.config.Config),
		// TODO: missing model.WithBoilerplate[From], needs boilerplate or path
		model.WithResource(resource, &api.config.Config),
	)
}

func (api *API) scaffoldV1() error {
	r := api.Resource

	if api.DoResource {
		fmt.Println(filepath.Join("pkg", "apis", r.Group, r.Version,
			fmt.Sprintf("%s_types.go", strings.ToLower(r.Kind))))
		fmt.Println(filepath.Join("pkg", "apis", r.Group, r.Version,
			fmt.Sprintf("%s_types_test.go", strings.ToLower(r.Kind))))

		universe, err := api.buildUniverse(r)
		if err != nil {
			return fmt.Errorf("error building API scaffold: %v", err)
		}

		err = (&Scaffold{}).Execute(
			universe,
			input.Options{},
			&crdv1.Register{Resource: r},
			&crdv1.Types{Resource: r},
			&crdv1.VersionSuiteTest{Resource: r},
			&crdv1.TypesTest{Resource: r},
			&crdv1.Doc{Resource: r},
			&crdv1.Group{Resource: r},
			&crdv1.AddToScheme{Resource: r},
			&crdv1.CRDSample{Resource: r},
		)
		if err != nil {
			return fmt.Errorf("error scaffolding APIs: %v", err)
		}
	} else {
		// disable generation of example reconcile body if not scaffolding resource
		// because this could result in a fork-bomb of k8s resources where watching a
		// deployment, replicaset etc. results in generating deployment which
		// end up generating replicaset, pod etc recursively.
		r.CreateExampleReconcileBody = false
	}

	if api.DoController {
		fmt.Println(filepath.Join("pkg", "controller", strings.ToLower(r.Kind),
			fmt.Sprintf("%s_controller.go", strings.ToLower(r.Kind))))
		fmt.Println(filepath.Join("pkg", "controller", strings.ToLower(r.Kind),
			fmt.Sprintf("%s_controller_test.go", strings.ToLower(r.Kind))))

		universe, err := api.buildUniverse(r)
		if err != nil {
			return fmt.Errorf("error building controller scaffold: %v", err)
		}

		err = (&Scaffold{}).Execute(
			universe,
			input.Options{},
			&controller.Controller{Resource: r},
			&controller.AddController{Resource: r},
			&controller.Test{Resource: r},
			&controller.SuiteTest{Resource: r},
		)
		if err != nil {
			return fmt.Errorf("error scaffolding controller: %v", err)
		}
	}

	return nil
}

func (api *API) scaffoldV2() error {
	r := api.Resource

	if api.DoResource {
		if err := api.validateResourceGroup(r); err != nil {
			return err
		}

		// Only save the resource in the config file if it didn't exist
		if api.config.AddResource(api.Resource) {
			if err := api.config.Save(); err != nil {
				return fmt.Errorf("error updating project file with resource information : %v", err)
			}
		}

		var path string
		if api.config.MultiGroup {
			path = filepath.Join("apis", r.Group, r.Version, fmt.Sprintf("%s_types.go", strings.ToLower(r.Kind)))
		} else {
			path = filepath.Join("api", r.Version, fmt.Sprintf("%s_types.go", strings.ToLower(r.Kind)))
		}
		fmt.Println(path)

		scaffold := &Scaffold{
			Plugins: api.Plugins,
		}

		universe, err := api.buildUniverse(r)
		if err != nil {
			return fmt.Errorf("error building API scaffold: %v", err)
		}

		files := []input.File{
			&scaffoldv2.Types{
				Input: input.Input{
					Path: path,
				},
				Resource: r},
			&scaffoldv2.Group{Resource: r},
			&scaffoldv2.CRDSample{Resource: r},
			&scaffoldv2.CRDEditorRole{Resource: r},
			&scaffoldv2.CRDViewerRole{Resource: r},
			&crdv2.EnableWebhookPatch{Resource: r},
			&crdv2.EnableCAInjectionPatch{Resource: r},
		}

		if err = scaffold.Execute(universe, input.Options{}, files...); err != nil {
			return fmt.Errorf("error scaffolding APIs: %v", err)
		}

		universe, err = api.buildUniverse(r)
		if err != nil {
			return fmt.Errorf("error building kustomization scaffold: %v", err)
		}

		crdKustomization := &crdv2.Kustomization{Resource: r}
		err = (&Scaffold{}).Execute(
			universe,
			input.Options{},
			crdKustomization,
			&crdv2.KustomizeConfig{},
		)
		if err != nil {
			return fmt.Errorf("error scaffolding kustomization: %v", err)
		}

		if err := crdKustomization.Update(); err != nil {
			return fmt.Errorf("error updating kustomization.yaml: %v", err)
		}

	} else {
		// disable generation of example reconcile body if not scaffolding resource
		// because this could result in a fork-bomb of k8s resources where watching a
		// deployment, replicaset etc. results in generating deployment which
		// end up generating replicaset, pod etc recursively.
		r.CreateExampleReconcileBody = false
	}

	if api.DoController {
		if api.config.MultiGroup {
			fmt.Println(filepath.Join("controllers", fmt.Sprintf("%s/%s_controller.go", r.Group, strings.ToLower(r.Kind))))
		} else {
			fmt.Println(filepath.Join("controllers", fmt.Sprintf("%s_controller.go", strings.ToLower(r.Kind))))
		}

		scaffold := &Scaffold{
			Plugins: api.Plugins,
		}

		universe, err := api.buildUniverse(r)
		if err != nil {
			return fmt.Errorf("error building controller scaffold: %v", err)
		}

		testsuiteScaffolder := &controllerv2.SuiteTest{Resource: r}
		err = scaffold.Execute(
			universe,
			input.Options{},
			testsuiteScaffolder,
			&controllerv2.Controller{Resource: r},
		)
		if err != nil {
			return fmt.Errorf("error scaffolding controller: %v", err)
		}

		err = testsuiteScaffolder.Update()
		if err != nil {
			return fmt.Errorf("error updating suite_test.go under controllers pkg: %v", err)
		}
	}

	err := (&scaffoldv2.Main{}).Update(
		&scaffoldv2.MainUpdateOptions{
			Config:         &api.config.Config,
			WireResource:   api.DoResource,
			WireController: api.DoController,
			Resource:       r,
		})
	if err != nil {
		return fmt.Errorf("error updating main.go: %v", err)
	}

	return nil
}

// isGroupAllowed will check if the group is == the group used before
// and not allow new groups if the project is not enabled to use multigroup layout
func (api *API) isGroupAllowed(r *resource.Resource) bool {
	if api.config.MultiGroup {
		return true
	}
	for _, existingGroup := range api.config.ResourceGroups() {
		if !strings.EqualFold(r.Group, existingGroup) {
			return false
		}
	}
	return true
}

// validateResourceGroup will return an error if the group cannot be created
func (api *API) validateResourceGroup(r *resource.Resource) error {
	if api.config.HasResource(api.Resource) && !api.Force {
		return fmt.Errorf("group '%s', version '%s' and kind '%s' already exists", r.Group, r.Version, r.Kind)
	}
	if !api.isGroupAllowed(r) {
		return fmt.Errorf("group '%s' is not same as existing group."+
			" Multiple groups are not enabled in this project. To enable, use the multigroup command", r.Group)
	}
	return nil
}
