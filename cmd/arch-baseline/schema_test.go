package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// schemaPath は measure skill が持つ出力の JSON schema。evaluate / remediate の references はこれを指す
const schemaPath = "../../skills/measure/references/baseline.schema.json"

// TestSchemaに出力が従う は、本物の analyze-modularity の出力を入れた JSON が schema に従うかを確かめる。
// schema にないキーを出したり、required のキーを落としたりしたら落ちる
func TestSchemaに出力が従う(t *testing.T) {
	schema := loadJSON(t, schemaPath)

	root := t.TempDir()
	files := map[string]string{
		"go.mod": `module example.com/schemafixture

go 1.26.1
`,
		"main.go": `package main

import "example.com/schemafixture/lib"

func main() { lib.Used() }
`,
		"lib/lib.go": `package lib

type Store interface{ Get() string }

type memory struct{}

func (memory) Get() string { return "" }

func Used() Store { return memory{} }

func Unused() {}
`,
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	modularity, err := exec.Command("go", "run", "../analyze-modularity", root).Output()
	if err != nil {
		t.Fatalf("analyze-modularity 失敗: %v", withStderr(err))
	}

	b := baseline{
		Project:      "example.com/schemafixture",
		GolangciLint: json.RawMessage(emptyGolangci),
		GoArchLint:   json.RawMessage(emptyGoArchLint),
		Gosec:        json.RawMessage(emptyGosec),
		Modularity:   modularity,
		Govulncheck:  vulnResult{"unmeasured", 1},
	}
	out := filepath.Join(t.TempDir(), "baseline.json")
	if err := writeJSON(out, b); err != nil {
		t.Fatal(err)
	}

	v := &validator{defs: asObject(schema["$defs"])}
	v.validate("$", schema, loadJSON(t, out))
	for _, e := range v.errs {
		t.Error(e)
	}
}

func loadJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return v
}

func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// validator は schema が使う範囲 ($ref、type、enum、required、properties、additionalProperties、items) だけを見る
type validator struct {
	defs map[string]any
	errs []string
}

func (v *validator) fail(path, format string, a ...any) {
	v.errs = append(v.errs, path+": "+fmt.Sprintf(format, a...))
}

func (v *validator) validate(path string, schema map[string]any, value any) {
	if ref, ok := schema["$ref"].(string); ok {
		schema = asObject(v.defs[strings.TrimPrefix(ref, "#/$defs/")])
	}
	if enum, ok := schema["enum"].([]any); ok && !slices.Contains(enum, value) {
		v.fail(path, "%v は %v のどれでもない", value, enum)
	}
	if typ, ok := schema["type"]; ok && !matchesType(typ, value) {
		v.fail(path, "%T は type %v に合わない", value, typ)
		return
	}
	switch val := value.(type) {
	case map[string]any:
		v.validateObject(path, schema, val)
	case []any:
		if items := asObject(schema["items"]); items != nil {
			for i, item := range val {
				v.validate(fmt.Sprintf("%s[%d]", path, i), items, item)
			}
		}
	}
}

func (v *validator) validateObject(path string, schema, obj map[string]any) {
	required, _ := schema["required"].([]any)
	for _, r := range required {
		if _, ok := obj[r.(string)]; !ok {
			v.fail(path, "required の %s が無い", r)
		}
	}
	props := asObject(schema["properties"])
	for key, val := range obj {
		sub, ok := props[key]
		if !ok {
			if schema["additionalProperties"] == false {
				v.fail(path, "schema に無いキー %s", key)
			}
			continue
		}
		v.validate(path+"."+key, asObject(sub), val)
	}
}

func matchesType(typ, value any) bool {
	if types, ok := typ.([]any); ok {
		return slices.ContainsFunc(types, func(t any) bool { return matchesType(t, value) })
	}
	switch typ {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		n, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := n.Int64()
		return err == nil
	}
	return false
}
