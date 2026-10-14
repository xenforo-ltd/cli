package devoutput_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

// TestExportEmptyOutput checks empty containers, result counts, XML framing,
// and permissions for missing and empty type directories.
func TestExportEmptyOutput(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"missing types", "empty directories", "empty metadata", "special files"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			switch kind {
			case "empty directories":
				for _, name := range []string{"templates/public", "templates/admin", "permissions"} {
					if err := os.MkdirAll(filepath.Join(root, "_output", filepath.FromSlash(name)), 0o755); err != nil {
						t.Fatal(err)
					}
				}
			case "empty metadata":
				writeFile(t, root, "_output/api_scopes/_metadata.jsonl", " \n\t\n")
			case "special files":
				for _, name := range []string{".ignored", ".hidden/file.txt", "extension_hint.php", "option_hint.php", "style_properties.less"} {
					writeFile(t, root, "_output/"+name, "ignored")
				}
			}
			result, err := devoutput.Export(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			want := devoutput.Result{Directory: filepath.Join(root, "_data"), Files: 27}
			if !reflect.DeepEqual(*result, want) {
				t.Fatalf("result = %+v, want %+v", result, want)
			}
			files, err := os.ReadDir(result.Directory)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != result.Files {
				t.Fatalf("file count = %d, want %d", len(files), result.Files)
			}
			for _, container := range []string{
				"activity_summary_definitions", "admin_navigation", "admin_permission", "advertising_positions",
				"api_scopes", "bb_codes", "bb_code_media_sites", "class_extensions", "code_events",
				"code_event_listeners", "content_type_fields", "cron", "help_pages", "member_stats",
				"navigation", "option_groups", "options", "permission_interface_groups", "permissions",
				"phrases", "routes", "style_properties", "style_property_groups", "template_modifications",
				"templates", "widget_definitions", "widget_positions",
			} {
				name := "_data/" + container + ".xml"
				data := readFile(t, root, name)
				want := "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<" + container + "></" + container + ">\n"
				if data != want {
					t.Errorf("%s = %q, want %q", name, data, want)
				}
				info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o644) {
					t.Errorf("%s mode = %v, want regular file with mode 0644", name, info.Mode())
				}
			}
		})
	}
}

// TestExportOrdering checks record-key ordering, numeric ordering, composite
// ordering, and filename tie-breaking independently of metadata declaration order.
func TestExportOrdering(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		directory string
		container string
		files     []sourceFile
		want      string
	}{
		{
			"admin_permissions", "admin_permission",
			[]sourceFile{
				{name: "z.json", content: `{"display_order":0}`},
				{name: "a.json", content: `{"display_order":1}`},
				{name: "m.json", content: `{"admin_permission_id":"explicit","display_order":2}`},
			},
			`<admin_permission>
				<admin_permission admin_permission_id="a" display_order="1"></admin_permission>
				<admin_permission admin_permission_id="explicit" display_order="2"></admin_permission>
				<admin_permission admin_permission_id="z" display_order="0"></admin_permission>
			</admin_permission>`,
		},
		{
			"class_extensions", "class_extensions",
			[]sourceFile{
				{name: "z.json", content: `{"from_class":"A","to_class":"B","execute_order":10,"active":true}`},
				{name: "a.json", content: `{"from_class":"A","to_class":"B","execute_order":2,"active":false}`},
				{name: "m.json", content: `{"from_class":"A","to_class":"B","execute_order":-1,"active":true}`},
				{name: "b.json", content: `{"from_class":"A","to_class":"B","execute_order":2,"active":true}`},
				{name: "c.json", content: `{"from_class":"A","to_class":"C","execute_order":0,"active":true}`},
				{name: "d.json", content: `{"from_class":"B","to_class":"A","execute_order":0,"active":true}`},
			},
			`<class_extensions>
				<extension from_class="A" to_class="B" execute_order="-1" active="1"></extension>
				<extension from_class="A" to_class="B" execute_order="2" active="0"></extension>
				<extension from_class="A" to_class="B" execute_order="2" active="1"></extension>
				<extension from_class="A" to_class="B" execute_order="10" active="1"></extension>
				<extension from_class="A" to_class="C" execute_order="0" active="1"></extension>
				<extension from_class="B" to_class="A" execute_order="0" active="1"></extension>
			</class_extensions>`,
		},
		{
			"content_type_fields", "content_type_fields",
			[]sourceFile{
				{name: "a.json", content: `{"content_type":"b","field_name":"a","field_value":"third"}`},
				{name: "b.json", content: `{"content_type":"a","field_name":"b","field_value":"second"}`},
				{name: "c.json", content: `{"content_type":"a","field_name":"a","field_value":"first"}`},
			},
			`<content_type_fields>
				<field content_type="a" field_name="a">first</field>
				<field content_type="a" field_name="b">second</field>
				<field content_type="b" field_name="a">third</field>
			</content_type_fields>`,
		},
	} {
		t.Run(tc.directory, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			writeOutput(t, root, tc.directory, tc.files...)
			for range 3 {
				result, err := devoutput.Export(t.Context(), root)
				if err != nil {
					t.Fatal(err)
				}
				if result.Entries != len(tc.files) {
					t.Fatalf("Entries = %d, want %d", result.Entries, len(tc.files))
				}
				requireXML(t, root, tc.container, tc.want)
			}
		})
	}
}

// TestExportScalarValues checks JSON scalar conversion, XML escaping, Unicode,
// and preservation of numeric precision and text whitespace.
func TestExportScalarValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"string", "  <value> & \"quotes\" 'apostrophes'\n\t\r😀  ", "  <value> & \"quotes\" 'apostrophes'\n\t\r😀  "},
		{"integer", json.Number("9007199254740993"), "9007199254740993"},
		{"number spelling", json.Number("1.250e+2"), "1.250e+2"},
		{"true", true, "1"},
		{"false", false, "0"},
		{"null", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			data, err := json.Marshal(map[string]any{"content_type": "demo", "field_name": "value", "field_value": tc.value})
			if err != nil {
				t.Fatal(err)
			}
			writeOutput(t, root, "content_type_fields", sourceFile{name: "demo.json", content: string(data)})
			if _, err := devoutput.Export(t.Context(), root); err != nil {
				t.Fatal(err)
			}
			node := parseXML(t, readFile(t, root, "_data/content_type_fields.xml"))
			if len(node.Children) != 1 || node.Children[0].Text != tc.want {
				t.Fatalf("exported scalar = %+v, want text %q", node.Children, tc.want)
			}
		})
	}
}

// TestExportEmptyValues checks omission of PHP-style empty values while retaining
// mapped false and zero attributes and required empty content.
func TestExportEmptyValues(t *testing.T) {
	t.Parallel()
	for _, empty := range []string{"null", "false", "0", `"0"`, `""`, "[ ]", "{ }"} {
		t.Run(empty, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			writeOutput(t, root, "code_events", sourceFile{name: "demo.json", content: `{"description":"","arguments":` + empty + `,"hint_description":` + empty + `}`})
			writeOutput(t, root, "options", sourceFile{name: "demo.json", content: `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"","edit_format_params":"","sub_options":` + empty + `,"relations":` + empty + `}`})
			writeOutput(t, root, "phrases", sourceFile{name: "demo.txt", metadata: map[string]any{"version_id": 0, "version_string": "", "global_cache": json.RawMessage(empty)}})
			writeOutput(t, root, "style_properties", sourceFile{name: "demo.json", content: `{"group_name":"","title":"Demo","description":"","property_type":"value","value_type":"string","has_variations":false,"depends_on":"","value_group":"","display_order":0,"css_components":` + empty + `,"value_parameters":"","property_value":null}`})
			result, err := devoutput.Export(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			if result.Entries != 4 {
				t.Fatalf("Entries = %d, want 4", result.Entries)
			}
			requireXML(t, root, "code_events", `<code_events><event event_id="demo"></event></code_events>`)
			requireXML(t, root, "options", `<options><option option_id="demo" edit_format="textbox" data_type="string" advanced="0"><default_value></default_value></option></options>`)
			requireXML(t, root, "phrases", `<phrases><phrase title="demo" version_id="0"></phrase></phrases>`)
			requireXML(t, root, "style_properties", `<style_properties><style_property property_name="demo" title="Demo" property_type="value" value_type="string" has_variations="0" display_order="0"><value>null</value></style_property></style_properties>`)
		})
	}
}

// TestExportCDATA checks that CDATA terminators are split without changing text.
func TestExportCDATA(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	content := "before ]]> <tag> & after\n"
	writeOutput(t, root, "templates", sourceFile{name: "public/demo.html", content: content, metadata: map[string]any{"version_id": 1, "version_string": "1.0"}})
	if _, err := devoutput.Export(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	data := readFile(t, root, "_data/templates.xml")
	if !strings.Contains(data, "<![CDATA[") || !strings.Contains(data, "]]><![CDATA[") {
		t.Fatalf("CDATA terminator was not split: %s", data)
	}
	node := parseXML(t, data)
	if len(node.Children) != 1 || node.Children[0].Text != content {
		t.Fatalf("template text = %+v, want %q", node.Children, content)
	}
}

// TestExportSelection checks that only metadata-listed files are exported and
// stale release XML is replaced after entries are removed.
func TestExportSelection(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	writeOutput(t, root, "api_scopes", sourceFile{name: "listed.json", content: `{}`})
	writeFile(t, root, "_output/api_scopes/unlisted.json", "invalid JSON")
	writeFile(t, root, "_data/notes.txt", "keep")
	writeFile(t, root, "_data/api_scopes.xml", "previous data")
	result, err := devoutput.Export(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Entries != 1 {
		t.Fatalf("Entries = %d, want 1", result.Entries)
	}
	requireXML(t, root, "api_scopes", `<api_scopes><api_scope api_scope_id="listed"></api_scope></api_scopes>`)
	writeOutput(t, root, "api_scopes")
	result, err = devoutput.Export(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Entries != 0 {
		t.Fatalf("Entries = %d, want 0", result.Entries)
	}
	requireXML(t, root, "api_scopes", `<api_scopes></api_scopes>`)
	if got := readFile(t, root, "_data/notes.txt"); got != "keep" {
		t.Fatal("export changed unrelated release data")
	}
}

// TestExportHashNormalization checks hashes prepared with LF against CRLF files.
func TestExportHashNormalization(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	content := "{\n  \"display_order\": 1\n}\n"
	writeOutput(t, root, "admin_permissions", sourceFile{name: "demo.json", content: content})
	writeFile(t, root, "_output/admin_permissions/demo.json", strings.ReplaceAll(content, "\n", "\r\n"))
	if _, err := devoutput.Export(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	requireXML(t, root, "admin_permission", `<admin_permission><admin_permission admin_permission_id="demo" display_order="1"></admin_permission></admin_permission>`)
}

// TestExportCancellation checks preservation of an already canceled context.
func TestExportCancellation(t *testing.T) {
	t.Parallel()
	root := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := devoutput.Export(ctx, root)
	if !errors.Is(err, context.Canceled) || result != nil {
		t.Fatalf("result = %+v, error = %v, want cancellation without a result", result, err)
	}
	requireMissing(t, root, "_data")
}

// TestExportRelativePath checks that a relative add-on path yields an absolute
// release directory. It cannot run in parallel because it changes the directory.
func TestExportRelativePath(t *testing.T) {
	root := newFixture(t)
	t.Chdir(filepath.Dir(root))
	result, err := devoutput.Export(t.Context(), filepath.Base(root))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "_data"); result.Directory != want {
		t.Fatalf("Directory = %q, want %q", result.Directory, want)
	}
}
