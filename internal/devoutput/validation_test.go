package devoutput_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

// TestExportInvalidMetadata checks malformed metadata records and unsafe,
// missing, and duplicate filenames through Export.
func TestExportInvalidMetadata(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{"invalid JSON", `{`, "decoding _metadata.jsonl"},
		{"invalid record type", `[]`, "decoding _metadata.jsonl"},
		{"null record", `null`, "metadata record has no filename"},
		{"missing filename", `{}`, "metadata record has no filename"},
		{"empty filename", `{"_file":""}`, "metadata record has no filename"},
		{"invalid filename type", `{"_file":1}`, "metadata record has no filename"},
		{"absolute filename", `{"_file":"/demo.json"}`, "invalid metadata filename"},
		{"parent filename", `{"_file":"../demo.json"}`, "invalid metadata filename"},
		{"backslash filename", `{"_file":"public\\demo.json"}`, "invalid metadata filename"},
		{"drive filename", `{"_file":"C:demo.json"}`, "invalid metadata filename"},
		{"empty path component", `{"_file":"public//demo.json"}`, "invalid metadata filename"},
		{"duplicate filename", "{\"_file\":\"demo.json\"}\n{\"_file\":\"demo.json\"}\n", "duplicate metadata for demo.json"},
		{"trailing invalid record", "{\"_file\":\"demo.json\"}\n{", "decoding _metadata.jsonl"},
		{"invalid UTF-8", "{\"_file\":\"\xff.json\"}", "_metadata.jsonl contains invalid UTF-8"},
		{"missing hash", `{"_file":"demo.json"}`, "stale metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			writeFile(t, root, "_output/api_scopes/demo.json", "{}")
			writeFile(t, root, "_output/api_scopes/_metadata.jsonl", tc.data)
			result, err := devoutput.Export(t.Context(), root)
			requireError(t, err, tc.want)
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
			requireMissing(t, root, "_data")
		})
	}
}

// TestExportRecordValidation checks that unknown fields warn and invalid known
// fields, hashes, filenames, or XML text leave existing release data intact.
func TestExportRecordValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		directory string
		file      sourceFile
		want      string
	}{
		{"stale hash", "api_scopes", sourceFile{name: "demo.json", content: `{}`, metadata: map[string]any{"hash": "wrong"}}, "stale metadata"},
		{"null hash", "api_scopes", sourceFile{name: "demo.json", content: `{}`, metadata: map[string]any{"hash": nil}}, "stale metadata"},
		{"invalid JSON", "api_scopes", sourceFile{name: "demo.json", content: `{`}, "reading _output/api_scopes/demo.json"},
		{"null JSON", "api_scopes", sourceFile{name: "demo.json", content: `null`}, "expected a JSON object"},
		{"JSON array", "api_scopes", sourceFile{name: "demo.json", content: `[1]`}, "reading _output/api_scopes/demo.json"},
		{"wrong suffix", "api_scopes", sourceFile{name: "demo.txt", content: `{}`}, "expected a JSON file"},
		{"unknown field", "api_scopes", sourceFile{name: "demo.json", content: `{"custom":1,"zzz":[],"aaa":null}`}, ""},
		{"missing attribute", "activity_summary_definitions", sourceFile{name: "demo.json", content: `{}`}, `missing field "definition_class"`},
		{"object attribute", "activity_summary_definitions", sourceFile{name: "demo.json", content: `{"definition_class":{}}`}, "expected a scalar"},
		{"missing composite key", "permissions", sourceFile{name: "demo.json", content: `{}`}, "must contain permission_group_id and permission_id"},
		{"missing template type", "templates", sourceFile{name: "demo.html", content: "text"}, "must contain type and title"},
		{"reserved text metadata", "phrases", sourceFile{name: "demo.txt", content: "text", metadata: map[string]any{"title": "override"}}, `unsupported metadata field "title"`},
		{"unknown text metadata", "phrases", sourceFile{name: "demo.txt", content: "text", metadata: map[string]any{"custom": true, "version_id": 1, "version_string": "1.0", "global_cache": false}}, ""},
		{"missing text version", "templates", sourceFile{name: "public/demo.html", content: "text", metadata: map[string]any{"version_id": 1}}, `missing field "version_string"`},
		{"invalid UTF-8", "api_scopes", sourceFile{name: "demo.json", content: "{\"api_scope_id\":\"\xff\"}"}, "contains invalid UTF-8"},
		{"invalid attribute character", "api_scopes", sourceFile{name: "demo.json", content: `{"api_scope_id":"bad\u0001"}`}, "attribute api_scope_id: character U+0001"},
		{"invalid attribute noncharacter", "api_scopes", sourceFile{name: "demo.json", content: `{"api_scope_id":"bad\ufffe"}`}, "character U+FFFE"},
		{"invalid text character", "phrases", sourceFile{name: "demo.txt", content: "bad\x00", metadata: map[string]any{"version_id": 1, "version_string": "1.0", "global_cache": false}}, "element phrase: character U+0000"},
		{"invalid content character", "content_type_fields", sourceFile{name: "demo.json", content: `{"content_type":"demo","field_name":"value","field_value":"bad\u0001"}`}, "element field: character U+0001"},
		{"missing content", "content_type_fields", sourceFile{name: "demo.json", content: `{"content_type":"demo","field_name":"value"}`}, `missing field "field_value"`},
		{"array content", "content_type_fields", sourceFile{name: "demo.json", content: `{"content_type":"demo","field_name":"value","field_value":[1]}`}, "expected a scalar"},
		{"invalid numeric order", "class_extensions", sourceFile{name: "demo.json", content: `{"from_class":"A","to_class":"B","execute_order":"bad","active":true}`}, "invalid numeric execute_order"},
		{"missing child field", "options", sourceFile{name: "demo.json", content: `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"","sub_options":[],"relations":[]}`}, `missing field "edit_format_params"`},
		{"missing repeated field", "options", sourceFile{name: "demo.json", content: `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"","edit_format_params":"","sub_options":[]}`}, `missing field "relations"`},
		{"invalid repeated field", "options", sourceFile{name: "demo.json", content: `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"","edit_format_params":"","sub_options":[],"relations":[1]}`}, `field "relations"`},
		{"invalid repeated value", "options", sourceFile{name: "demo.json", content: `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"","edit_format_params":"","sub_options":[],"relations":{"demo":{}}}`}, `field "$value"`},
		{"invalid line values", "options", sourceFile{name: "demo.json", content: `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"","edit_format_params":"","sub_options":[1],"relations":[]}`}, `field "sub_options"`},
		{"invalid comma values", "style_properties", sourceFile{name: "demo.json", content: `{"group_name":"","title":"Demo","description":"","property_type":"css","value_type":"css","has_variations":false,"depends_on":"","value_group":"","display_order":0,"css_components":[1],"value_parameters":"","property_value":null}`}, `field "css_components"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			files := []sourceFile{tc.file}
			if tc.name == "unknown field" {
				other := tc.file
				other.name = "another.json"
				files = append(files, other)
			}
			writeOutput(t, root, tc.directory, files...)
			writeFile(t, root, "_data/activity_summary_definitions.xml", "previous data")
			result, err := devoutput.Export(t.Context(), root)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				want := []string{tc.directory + `: ignoring unknown field "custom"`}
				if tc.name == "unknown field" {
					want = []string{
						`api_scopes: ignoring unknown field "aaa"`,
						`api_scopes: ignoring unknown field "custom"`,
						`api_scopes: ignoring unknown field "zzz"`,
					}
					requireXML(t, root, "api_scopes", `<api_scopes><api_scope api_scope_id="another"></api_scope><api_scope api_scope_id="demo"></api_scope></api_scopes>`)
				} else {
					requireXML(t, root, "phrases", `<phrases><phrase title="demo" version_id="1" version_string="1.0"><![CDATA[text]]></phrase></phrases>`)
				}
				if !slices.Equal(result.Warnings, want) {
					t.Fatalf("Warnings = %q, want %q", result.Warnings, want)
				}
				return
			}
			requireError(t, err, tc.want)
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
			if got := readFile(t, root, "_data/activity_summary_definitions.xml"); got != "previous data" {
				t.Fatal("validation failure replaced existing release data")
			}
			requireMissing(t, root, "_data/admin_navigation.xml")
		})
	}
}

// TestExportCustomHandlers checks rejection of export-handler class extensions,
// including disabled extensions and case-insensitive names with leading slashes.
func TestExportCustomHandlers(t *testing.T) {
	t.Parallel()
	for _, class := range []string{
		`XF\AddOn\DataManager`,
		`\xF\aDdOn\dAtAtYpE\Phrase`,
		`\\XF\DevelopmentOutput\Template`,
	} {
		t.Run(class, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			data, err := json.Marshal(map[string]any{"from_class": class, "to_class": "Vendor\\Custom", "execute_order": 10, "active": false})
			if err != nil {
				t.Fatal(err)
			}
			writeOutput(t, root, "class_extensions", sourceFile{name: "demo.json", content: string(data)})
			result, err := devoutput.Export(t.Context(), root)
			requireError(t, err, "custom export handler")
			if result != nil {
				t.Fatalf("failed export returned result: %+v", result)
			}
			requireMissing(t, root, "_data")
		})
	}
}
