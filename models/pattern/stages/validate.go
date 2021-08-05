package stages

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/layer5io/meshery/models/pattern/core"
	"github.com/layer5io/meshery/models/pattern/resource/selector"
	"github.com/layer5io/meshkit/models/oam/core/v1alpha1"
	"github.com/qri-io/jsonschema"
)

func Validator(prov ServiceInfoProvider, act ServiceActionProvider) ChainStageFunction {
	s := selector.New(prov)

	return func(data *Data, err error, next ChainStageNextFunction) {
		data.PatternSvcWorkloadCapabilities = map[string]core.WorkloadCapability{}
		data.PatternSvcTraitCapabilities = map[string][]core.TraitCapability{}

		for svcName, svc := range data.Pattern.Services {
			wc, ok := s.Workload(svc.Type)
			if !ok {
				act.Terminate(fmt.Errorf("invalid workload of type: %s", svc.Type))
				return
			}

			// Validate workload definition
			if err := validateWorkload(svc.Settings, wc); err != nil {
				act.Terminate(fmt.Errorf("invalid workload definition: %s", err))
				return
			}

			// Store the workload capability in the metadata
			data.PatternSvcWorkloadCapabilities[svcName] = wc

			data.PatternSvcTraitCapabilities[svcName] = []core.TraitCapability{}

			// Validate traits applied to this workload
			for trName, tr := range svc.Traits {
				tc, ok := s.Trait(trName)
				if !ok {
					act.Terminate(fmt.Errorf("invalid trait of type: %s", svc.Type))
					return
				}

				if err := validateTrait(tr, tc, svc.Type); err != nil {
					act.Terminate(err)
					return
				}

				// Store the trait capability in the metadata
				data.PatternSvcTraitCapabilities[svcName] = append(data.PatternSvcTraitCapabilities[svcName], tc)
			}
		}

		if next != nil {
			next(data, nil)
		}
	}
}

func validateWorkload(comp map[string]interface{}, wc core.WorkloadCapability) error {
	// Create schema validator from the schema
	rs := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(wc.OAMRefSchema), rs); err != nil {
		return fmt.Errorf("failed to create schema: %s", err)
	}

	// Create json settings
	jsonSettings, err := json.Marshal(comp)
	if err != nil {
		return fmt.Errorf("failed to generate schema from the PatternFile settings: %s", err)
	}

	// Validate the json against the schema
	errs, err := rs.ValidateBytes(context.TODO(), jsonSettings)
	if err != nil {
		return fmt.Errorf("error occurred during schema validation: %s", err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid settings: %s", errs)
	}

	return nil
}

func validateTrait(trait interface{}, tc core.TraitCapability, compType string) error {
	// Create schema validator from the schema
	rs := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(tc.OAMRefSchema), rs); err != nil {
		return fmt.Errorf("failed to create schema: %s", err)
	}

	// Check if the trait applied to the component is legal or not
	isLegal := _isLegalTrait(tc, compType)
	if !isLegal {
		return fmt.Errorf(
			"%s trait is not applicable to %s",
			tc.OAMDefinition.Name,
			compType,
		)
	}

	// Create json of the trait's properties for validation
	jsonCompTraitProp, err := json.Marshal(trait)
	if err != nil {
		return fmt.Errorf(
			"failed to generate schema from the PatternFile's service type %s trait: %s",
			compType,
			err,
		)
	}

	// Validate the json against the schema
	errs, err := rs.ValidateBytes(context.TODO(), jsonCompTraitProp)
	if err != nil {
		return fmt.Errorf("error occurred during schema validation: %s", err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid traits: %s", errs)
	}

	return nil
}

func _isLegalTrait(tc core.TraitCapability, compType string) bool {
	if len(tc.OAMDefinition.Spec.AppliesToWorkloads) == 0 {
		return true
	}

	for _, wl := range tc.OAMDefinition.Spec.AppliesToWorkloads {
		if wl == compType {
			return true
		}
	}

	return false
}

// ValidateWorkload takes in a workload and validates it against the corresponding schema
func ValidateWorkload(workload interface{}, component v1alpha1.Component) (*core.WorkloadCapability, error) {
	castedWorklod, ok := workload.(core.WorkloadCapability)
	if !ok {
		return nil, fmt.Errorf("instance is not of type WorkloadCapability")
	}

	// Create schema validator from the schema
	rs := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(castedWorklod.OAMRefSchema), rs); err != nil {
		return &castedWorklod, fmt.Errorf("failed to create schema: %s", err)
	}

	// Create json settings
	jsonSettings, err := json.Marshal(component.Spec.Settings)
	if err != nil {
		return &castedWorklod, fmt.Errorf("failed to generate schema from the PatternFile settings: %s", err)
	}

	// Validate the json against the schema
	errs, err := rs.ValidateBytes(context.TODO(), jsonSettings)
	if err != nil {
		return &castedWorklod, fmt.Errorf("error occurred during schema validation: %s", err)
	}
	if len(errs) > 0 {
		return &castedWorklod, fmt.Errorf("invalid settings: %s", errs)
	}

	return &castedWorklod, nil
}

// ValidateTrait takes in a trait and a component and checks if the given trait is legal on that component
// or not. The function ASSUMES that if this function is called with a trait and corresponding component then
// the trait DOES EXIST in the ApplicationConfiguration
//
// After checking if the applied trait is legal or not it will validate the schema of the trait
func ValidateTrait(
	trait interface{},
	configSpecComp v1alpha1.ConfigurationSpecComponent,
	af core.Pattern,
) (*core.TraitCapability, error) {
	castedTrait, ok := trait.(core.TraitCapability)
	if !ok {
		return nil, fmt.Errorf("instance is not of type TraitCapability")
	}

	// Create schema validator from the schema
	rs := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(castedTrait.OAMRefSchema), rs); err != nil {
		return &castedTrait, fmt.Errorf("failed to create schema: %s", err)
	}

	// Check if the trait applied to the component is legal or not
	compTrait, isLegal := isLegalTrait(castedTrait, configSpecComp, af)
	if !isLegal {
		return &castedTrait, fmt.Errorf(
			"%s trait is not applicable to %s",
			castedTrait.OAMDefinition.Name,
			configSpecComp.ComponentName,
		)
	}

	// Create json of the trait's properties for validation
	jsonCompTraitProp, err := json.Marshal(compTrait.Properties)
	if err != nil {
		return &castedTrait, fmt.Errorf(
			"failed to generate schema from the PatternFile's service type %s trait: %s",
			configSpecComp.ComponentName,
			err,
		)
	}

	// Validate the json against the schema
	errs, err := rs.ValidateBytes(context.TODO(), jsonCompTraitProp)
	if err != nil {
		return &castedTrait, fmt.Errorf("error occurred during schema validation: %s", err)
	}
	if len(errs) > 0 {
		return &castedTrait, fmt.Errorf("invalid traits: %s", errs)
	}

	return &castedTrait, nil
}

func isLegalTrait(
	trait core.TraitCapability,
	configSpecComp v1alpha1.ConfigurationSpecComponent,
	af core.Pattern,
) (v1alpha1.ConfigurationSpecComponentTrait, bool) {
	for _, tr := range configSpecComp.Traits {
		if tr.Name == trait.OAMDefinition.Name {
			// If the slice is empty then according to the OAM spec the trait is assumed
			// to be applicable to every workload
			if len(trait.OAMDefinition.Spec.AppliesToWorkloads) == 0 {
				return tr, true
			}

			// Check if it's applicable
			for _, validType := range trait.OAMDefinition.Spec.AppliesToWorkloads {
				// Found the match
				if af.GetServiceType(configSpecComp.ComponentName) == validType {
					return tr, true
				}
			}

			return v1alpha1.ConfigurationSpecComponentTrait{}, false
		}
	}

	return v1alpha1.ConfigurationSpecComponentTrait{}, false
}
