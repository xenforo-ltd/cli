package devoutput_test

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

func prepared(t *testing.T, root, kind, name, content string) {
	t.Helper()
	write(t, root, "_output/"+kind+"/"+name, content)
	write(t, root, "_output/"+kind+"/_metadata.jsonl", fmt.Sprintf(`{"_file":%q,"hash":"%x"}`, name, md5.Sum([]byte(strings.ReplaceAll(content, "\r", "")))))
}

func TestExportNestedOptionsAndJSON(t *testing.T) {
	root := t.TempDir()
	prepared(t, root, "options", "demo.json", `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"<default>&","edit_format_params":"","sub_options":["alpha","beta"],"relations":{"zeta":20,"alpha":5}}`)
	prepared(t, root, "navigation", "nav.json", `{"parent_navigation_id":"","display_order":10,"navigation_type_id":"basic","enabled":true,"type_config":{"nested":["<tag>",{"value":2}]}}`)
	text := "Before ]]> <tag> & after"
	prepared(t, root, "phrases", "example.txt", text)
	// Text records retain version metadata in addition to the file hash.
	write(t, root, "_output/phrases/_metadata.jsonl", fmt.Sprintf(`{"_file":"example.txt","hash":"%x","version_id":1,"version_string":"1","global_cache":false}`, md5.Sum([]byte(text))))
	if _, err := devoutput.Export(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "_data/options.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var options struct {
		Entries []struct {
			ID        string  `xml:"option_id,attr"`
			Default   string  `xml:"default_value"`
			Params    *string `xml:"edit_format_params"`
			Sub       string  `xml:"sub_options"`
			Relations []struct {
				Group string `xml:"group_id,attr"`
				Order int    `xml:"display_order,attr"`
			} `xml:"relation"`
		} `xml:"option"`
	}
	if err := xml.Unmarshal(data, &options); err != nil || len(options.Entries) != 1 {
		t.Fatalf("options: %s, %v", data, err)
	}
	option := options.Entries[0]
	relations := make(map[string]int)
	for _, relation := range option.Relations {
		relations[relation.Group] = relation.Order
	}
	if option.ID != "demo" || option.Default != "<default>&" || option.Params != nil || option.Sub != "alpha\nbeta" || len(option.Relations) != 2 || !reflect.DeepEqual(relations, map[string]int{"alpha": 5, "zeta": 20}) {
		t.Fatalf("option: %+v", option)
	}
	data, err = os.ReadFile(filepath.Join(root, "_data/navigation.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var navigation struct {
		Entries []struct {
			Content string `xml:",chardata"`
		} `xml:"navigation_entry"`
	}
	if err := xml.Unmarshal(data, &navigation); err != nil || len(navigation.Entries) != 1 {
		t.Fatalf("navigation: %s, %v", data, err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(navigation.Entries[0].Content), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"nested":["<tag>",{"value":2}]}`), &want); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("navigation JSON: %#v, %v", got, err)
	}
	data, err = os.ReadFile(filepath.Join(root, "_data/phrases.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var phrases struct {
		Entries []struct {
			Text string `xml:",chardata"`
		} `xml:"phrase"`
	}
	if err := xml.Unmarshal(data, &phrases); err != nil || len(phrases.Entries) != 1 || phrases.Entries[0].Text != text || !strings.Contains(string(data), "<![CDATA[") {
		t.Fatalf("phrase CDATA: %s, %v", data, err)
	}
}

func TestExportRejectsInvalidContentAndCollections(t *testing.T) {
	base := `{"edit_format":"textbox","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"ok","edit_format_params":"","sub_options":["one"],"relations":{"group":1}}`
	for _, tc := range []struct{ name, kind, filename, content, message string }{
		{"invalid UTF-8", "phrases", "bad.txt", "bad\xff", "UTF-8"},
		{"invalid XML character", "options", "bad.json", strings.Replace(base, `"default_value":"ok"`, `"default_value":"\u0001"`, 1), "not valid in XML"},
		{"relations not a map", "options", "bad.json", strings.Replace(base, `"relations":{"group":1}`, `"relations":[1]`, 1), "relations"},
		{"relation value not scalar", "options", "bad.json", strings.Replace(base, `"relations":{"group":1}`, `"relations":{"group":{"nested":1}}`, 1), "scalar"},
		{"sub options not strings", "options", "bad.json", strings.Replace(base, `"sub_options":["one"]`, `"sub_options":[1]`, 1), "sub_options"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			prepared(t, root, tc.kind, tc.filename, tc.content)
			write(t, root, "_data/options.xml", "existing")
			if _, err := devoutput.Export(context.Background(), root); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(root, "_data/options.xml"))
			if err != nil || string(data) != "existing" {
				t.Fatalf("existing output: %q, %v", data, err)
			}
		})
	}
}
