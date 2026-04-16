package template

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	gt "text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
	templateutils "github.com/opendatahub-io/odh-platform-utilities/pkg/template"
)

// TemplateSource identifies a Go template to render. It embeds the
// filesystem and glob path, plus optional per-source label/annotation
// overrides.
type TemplateSource struct {
	Labels      map[string]string
	Annotations map[string]string
	FS          fs.FS
	Path        string
}

// Render renders Go text/template files from the given sources using the
// provided data map and returns the resulting unstructured resources.
//
// The scheme is used to build a runtime.Decoder for parsing rendered
// YAML. If scheme is nil, an empty scheme is used (sufficient for
// unstructured decoding).
func Render(
	_ context.Context,
	scheme *runtime.Scheme,
	sources []TemplateSource,
	data map[string]any,
	opts ...Option,
) ([]unstructured.Unstructured, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	o := options{
		labels:      make(map[string]string),
		annotations: make(map[string]string),
		funcMap:     templateutils.TextTemplateFuncMap(),
	}

	for _, opt := range opts {
		opt(&o)
	}

	if scheme == nil {
		scheme = runtime.NewScheme()
	}

	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	result := make([]unstructured.Unstructured, 0)

	var buffer bytes.Buffer

	for i := range sources {
		rendered, err := renderSource(decoder, &buffer, sources[i], data, &o)
		if err != nil {
			return nil, err
		}

		result = append(result, rendered...)
	}

	return result, nil
}

func renderSource(
	decoder runtime.Decoder,
	buffer *bytes.Buffer,
	source TemplateSource,
	data map[string]any,
	o *options,
) ([]unstructured.Unstructured, error) {
	tmpl, err := gt.New("").Option("missingkey=error").Funcs(o.funcMap).ParseFS(source.FS, source.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template from: %w", err)
	}

	var result []unstructured.Unstructured

	for _, t := range tmpl.Templates() {
		buffer.Reset()

		err = t.Execute(buffer, data)
		if err != nil {
			return nil, fmt.Errorf("failed to execute template: %w", err)
		}

		u, err := decodeRendered(decoder, buffer.Bytes(), o.labels, o.annotations, source)
		if err != nil {
			return nil, fmt.Errorf("failed to decode template: %w", err)
		}

		result = append(result, u...)
	}

	return result, nil
}

func decodeRendered(
	decoder runtime.Decoder,
	data []byte,
	globalLabels, globalAnnotations map[string]string,
	info TemplateSource,
) ([]unstructured.Unstructured, error) {
	u, err := resources.Decode(decoder, data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode template: %w", err)
	}

	for i := range u {
		resources.SetLabels(&u[i], globalLabels)
		resources.SetAnnotations(&u[i], globalAnnotations)

		resources.SetLabels(&u[i], info.Labels)
		resources.SetAnnotations(&u[i], info.Annotations)
	}

	return u, err
}

// buildScheme returns the scheme from the client, or nil.
func buildScheme(c client.Client) *runtime.Scheme {
	if c == nil {
		return nil
	}

	return c.Scheme()
}

// ComponentData wraps a client.Object to expose common metadata fields for
// use in Go templates. Templates can use {{.Component.Name}},
// {{.Component.Namespace}}, etc. without needing to call Go methods.
type ComponentData struct {
	Labels      map[string]string
	Annotations map[string]string
	Object      client.Object
	Name        string
	Namespace   string
}

// newComponentData wraps a client.Object into a template-friendly struct.
func newComponentData(obj client.Object) ComponentData {
	return ComponentData{
		Name:        obj.GetName(),
		Namespace:   obj.GetNamespace(),
		Labels:      obj.GetLabels(),
		Annotations: obj.GetAnnotations(),
		Object:      obj,
	}
}

// buildData builds the template data map from static data + dynamic
// data fns + the component instance and namespace.
func buildData(
	ctx context.Context, o *actionOpts, instance client.Object,
) (map[string]any, error) {
	data := maps.Clone(o.data)

	for _, fn := range o.dataFn {
		values, err := fn(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to compute template data: %w", err)
		}

		maps.Copy(data, values)
	}

	data[ComponentKey] = newComponentData(instance)

	if o.namespace != "" {
		data[AppNamespaceKey] = o.namespace
	}

	if o.nsFn != nil {
		ns, err := o.nsFn(ctx)
		if err != nil {
			return nil, err
		}

		data[AppNamespaceKey] = ns
	}

	return data, nil
}
