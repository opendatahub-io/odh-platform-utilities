package integration

// Management state constants — mirrors common.Managed and common.Removed as plain strings
// for use in map[string]any DSC specs.
const (
	Managed = "Managed"
	Removed = "Removed"
)

// Component name constants matching DSC spec field names. Convenience only:
// any string that is a field on the installed operator's DSC CRD works.
// A string that is not on that CRD is rejected by the API; this package
// does not onboard components.
const (
	CodeFlare            = "codeflare"
	Dashboard            = "dashboard"
	DataSciencePipelines = "datasciencepipelines"
	FeastOperator        = "feastoperator"
	Kserve               = "kserve"
	Kueue                = "kueue"
	ModelMeshServing     = "modelmeshserving"
	ModelRegistry        = "modelregistry"
	Ray                  = "ray"
	TrainingOperator     = "trainingoperator"
	TrustyAI             = "trustyai"
	Workbenches          = "workbenches"
)

// DSCSpec builds a DataScienceCluster spec. Chain Component() calls, end with ToMap().
type DSCSpec struct {
	components map[string]any
}

// NewDSCSpec creates an empty spec builder.
func NewDSCSpec() *DSCSpec {
	return &DSCSpec{components: make(map[string]any)}
}

// Component adds a component by DSC API field name. State is typically
// [Managed] or [Removed]. Name must already exist on the installed
// operator's DataScienceCluster CRD.
func (s *DSCSpec) Component(name, state string, opts ...func(map[string]any)) *DSCSpec {
	spec := map[string]any{"managementState": state}
	for _, opt := range opts {
		opt(spec)
	}
	s.components[name] = spec
	return s
}

// ToMap returns the spec as map[string]any for unstructured APIs.
func (s *DSCSpec) ToMap() map[string]any {
	if len(s.components) == 0 {
		return nil
	}
	return map[string]any{"components": s.components}
}

// Sub adds a nested sub-component (e.g., kserve's nim).
func Sub(name, state string) func(map[string]any) {
	return func(spec map[string]any) {
		spec[name] = map[string]any{"managementState": state}
	}
}

// Set adds a field to the component spec.
func Set(key string, value any) func(map[string]any) {
	return func(spec map[string]any) {
		spec[key] = value
	}
}
