package devoutput_test

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

// TestExportTypes checks the source fields, filenames, and XML shape of every
// built-in data type, including ignored fields and nested elements.
func TestExportTypes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		directory string
		container string
		file      sourceFile
		want      string
	}{
		{
			"activity_summary_definitions", "activity_summary_definitions",
			sourceFile{name: "demo.json", content: `{"definition_class":"Vendor\\Summary"}`},
			`<activity_summary_definition definition_id="demo" definition_class="Vendor\Summary"></activity_summary_definition>`,
		},
		{
			"admin_navigation", "admin_navigation",
			sourceFile{name: "demo.json", content: `{"parent_navigation_id":"","display_order":2,"link":"tools/demo?x=1&y=2","icon":"fa-test","admin_permission_id":"","debug_only":false,"development_only":true,"hide_no_children":false,"super_admin_only":true}`},
			`<admin_navigation_entry navigation_id="demo" display_order="2" link="tools/demo?x=1&amp;y=2" icon="fa-test" debug_only="0" development_only="1" hide_no_children="0"></admin_navigation_entry>`,
		},
		{
			"admin_permissions", "admin_permission",
			sourceFile{name: "demo.json", content: `{"display_order":1}`},
			`<admin_permission admin_permission_id="demo" display_order="1"></admin_permission>`,
		},
		{
			"advertising_positions", "advertising_positions",
			sourceFile{name: "demo.json", content: `{"active":true,"arguments":{ "slot": 2 }}`},
			`<advertising_position position_id="demo" active="1"><![CDATA[{"slot":2}]]></advertising_position>`,
		},
		{
			"api_scopes", "api_scopes",
			sourceFile{name: "demo.json", content: `{"usable_with_oauth_clients":true}`},
			`<api_scope api_scope_id="demo"></api_scope>`,
		},
		{
			"bb_codes", "bb_codes",
			sourceFile{name: "demo.json", content: `{"bb_code_mode":"replace","has_option":false,"callback_class":"","callback_method":"","option_regex":"","trim_lines_after":0,"plain_children":false,"disable_smilies":false,"disable_nl2br":true,"disable_autolink":false,"allow_empty":true,"allow_signature":false,"editor_icon_type":"fa","editor_icon_value":"fa-code","active":true,"replace_html":"<b>{text}</b>","replace_html_email":"<strong>{text}</strong>","replace_text":"{text}"}`},
			`<bb_code bb_code_id="demo" bb_code_mode="replace" has_option="0" trim_lines_after="0" plain_children="0" disable_smilies="0" disable_nl2br="1" disable_autolink="0" allow_empty="1" allow_signature="0" editor_icon_type="fa" editor_icon_value="fa-code" active="1">
				<replace_html><![CDATA[<b>{text}</b>]]></replace_html>
				<replace_html_email><![CDATA[<strong>{text}</strong>]]></replace_html_email>
				<replace_text><![CDATA[{text}]]></replace_text>
			</bb_code>`,
		},
		{
			"bb_code_media_sites", "bb_code_media_sites",
			sourceFile{name: "demo.json", content: `{"site_title":"Demo","site_url":"https://example.com","match_is_regex":false,"match_callback_class":"","match_callback_method":"","embed_html_callback_class":"","embed_html_callback_method":"","cookie_third_parties":"","supported":true,"active":false,"oembed_enabled":false,"oembed_api_endpoint":"","oembed_url_scheme":"","oembed_retain_scripts":false,"match_urls":"https://example.com/*","embed_html":"<iframe></iframe>"}`},
			`<site media_site_id="demo" site_title="Demo" site_url="https://example.com" match_is_regex="0" supported="1" active="0" oembed_enabled="0" oembed_retain_scripts="0">
				<match_urls><![CDATA[https://example.com/*]]></match_urls>
				<embed_html><![CDATA[<iframe></iframe>]]></embed_html>
			</site>`,
		},
		{
			"class_extensions", "class_extensions",
			sourceFile{name: "demo.json", content: `{"from_class":"XF\\Entity\\User","to_class":"Vendor\\User","execute_order":10,"active":true}`},
			`<extension from_class="XF\Entity\User" to_class="Vendor\User" execute_order="10" active="1"></extension>`,
		},
		{
			"code_events", "code_events",
			sourceFile{name: "demo.json", content: `{"description":"A <description>","arguments":[ "user" ],"hint_description":"Use a hint","is_deprecated":true}`},
			`<event event_id="demo" is_deprecated="1" arguments='["user"]' hint_description="Use a hint"><![CDATA[A <description>]]></event>`,
		},
		{
			"code_event_listeners", "code_event_listeners",
			sourceFile{name: "demo.json", content: `{"event_id":"demo","execute_order":10,"callback_class":"Vendor\\Listener","callback_method":"listen","active":true,"hint":"","description":""}`},
			`<listener event_id="demo" execute_order="10" callback_class="Vendor\Listener" callback_method="listen" active="1"></listener>`,
		},
		{
			"content_type_fields", "content_type_fields",
			sourceFile{name: "demo.json", content: `{"content_type":"demo","field_name":"handler_class","field_value":"Vendor\\Handler"}`},
			`<field content_type="demo" field_name="handler_class">Vendor\Handler</field>`,
		},
		{
			"cron_entries", "cron",
			sourceFile{name: "demo.json", content: `{"cron_class":"Vendor\\Cron","cron_method":"run","active":true,"run_rules":{ "minutes": [0,30], "hours": [12] }}`},
			`<entry entry_id="demo" cron_class="Vendor\Cron" cron_method="run" active="1"><![CDATA[{"minutes":[0,30],"hours":[12]}]]></entry>`,
		},
		{
			"help_pages", "help_pages",
			sourceFile{name: "demo.json", content: `{"page_name":"Demo","display_order":10,"callback_class":"","callback_method":"","advanced_mode":false,"active":true}`},
			`<help_page page_id="demo" page_name="Demo" display_order="10" advanced_mode="0" active="1"></help_page>`,
		},
		{
			"member_stats", "member_stats",
			sourceFile{name: "demo.json", content: `{"callback_class":"Vendor\\Stats","callback_method":"count","visibility_class":"","visibility_method":"","sort_order":"message_count","sort_direction":"desc","permission_limit":0,"show_value":true,"user_limit":20,"display_order":10,"overview_display":false,"active":true,"cache_lifetime":3600,"criteria":{ "user_state": "valid" }}`},
			`<member_stat member_stat_key="demo" callback_class="Vendor\Stats" callback_method="count" sort_order="message_count" sort_direction="desc" permission_limit="0" show_value="1" user_limit="20" display_order="10" overview_display="0" active="1" cache_lifetime="3600"><![CDATA[{"user_state":"valid"}]]></member_stat>`,
		},
		{
			"navigation", "navigation",
			sourceFile{name: "demo.json", content: `{"parent_navigation_id":"","display_order":1,"navigation_type_id":"basic","enabled":true,"type_config":{ "link": "demo/" }}`},
			`<navigation_entry navigation_id="demo" display_order="1" navigation_type_id="basic" enabled="1"><![CDATA[{"link":"demo/"}]]></navigation_entry>`,
		},
		{
			"option_groups", "option_groups",
			sourceFile{name: "demo.json", content: `{"icon":"fa-test","display_order":1,"debug_only":false,"advanced":true}`},
			`<group group_id="demo" icon="fa-test" display_order="1" debug_only="0"></group>`,
		},
		{
			"options", "options",
			sourceFile{name: "demo.json", content: `{"edit_format":"select","data_type":"string","validation_class":"","validation_method":"","advanced":false,"default_value":"<default>","edit_format_params":"0","sub_options":["a","b"],"relations":{"z":20,"a":10}}`},
			`<option option_id="demo" edit_format="select" data_type="string" advanced="0">
				<default_value>&lt;default&gt;</default_value>
				<edit_format_params>0</edit_format_params>
				<sub_options>a
b</sub_options>
				<relation group_id="a" display_order="10"></relation>
				<relation group_id="z" display_order="20"></relation>
			</option>`,
		},
		{
			"permission_interface_groups", "permission_interface_groups",
			sourceFile{name: "demo.json", content: `{"display_order":1,"is_moderator":false}`},
			`<interface_group interface_group_id="demo" display_order="1" is_moderator="0"></interface_group>`,
		},
		{
			"permissions", "permissions",
			sourceFile{name: "general-view.json", content: `{"permission_type":"flag","depend_permission_id":"","interface_group_id":"general","display_order":1}`},
			`<permission permission_group_id="general" permission_id="view" permission_type="flag" interface_group_id="general" display_order="1"></permission>`,
		},
		{
			"phrases", "phrases",
			sourceFile{name: "demo.txt", content: "  Welcome <user> & friends!  ", metadata: map[string]any{"version_id": json.Number("9007199254740993"), "version_string": "1.0", "global_cache": true}},
			`<phrase title="demo" version_id="9007199254740993" version_string="1.0" global_cache="1"><![CDATA[  Welcome <user> & friends!  ]]></phrase>`,
		},
		{
			"routes", "routes",
			sourceFile{name: "demo.json", content: `{"route_type":"public","route_prefix":"demo","sub_name":"","format":":int","build_class":"","build_method":"","controller":"Vendor\\Demo","context":"demo","action_prefix":""}`},
			`<route route_type="public" route_prefix="demo" format=":int" controller="Vendor\Demo" context="demo"></route>`,
		},
		{
			"style_properties", "style_properties",
			sourceFile{name: "demo.json", content: `{"group_name":"demo","title":"Demo","description":"","property_type":"css","value_type":"css","has_variations":false,"depends_on":"","value_group":"","display_order":1,"css_components":["background","border"],"value_parameters":"a=b","property_value":{"color":"red"}}`},
			`<style_property property_name="demo" group_name="demo" title="Demo" property_type="css" value_type="css" has_variations="0" display_order="1" css_components="background,border">
				<value_parameters>a=b</value_parameters>
				<value>{
    "color": "red"
}</value>
			</style_property>`,
		},
		{
			"style_property_groups", "style_property_groups",
			sourceFile{name: "demo.json", content: `{"title":"Demo","description":"","display_order":1}`},
			`<style_property_group group_name="demo" title="Demo" display_order="1"></style_property_group>`,
		},
		{
			"template_modifications", "template_modifications",
			sourceFile{name: "public/demo.json", content: `{"template":"page_container","description":"","execution_order":10,"enabled":true,"action":"str_replace","find":"<div>","replace":"<main>"}`},
			`<modification type="public" template="page_container" modification_key="demo" execution_order="10" enabled="1" action="str_replace">
				<find><![CDATA[<div>]]></find>
				<replace><![CDATA[<main>]]></replace>
			</modification>`,
		},
		{
			"templates", "templates",
			sourceFile{name: "public/demo.html", content: "<xf:if is=\"$user\">Hello 😀</xf:if>\n", metadata: map[string]any{"version_id": 100, "version_string": "1.0"}},
			"<template type=\"public\" title=\"demo\" version_id=\"100\" version_string=\"1.0\"><![CDATA[<xf:if is=\"$user\">Hello 😀</xf:if>\n]]></template>",
		},
		{
			"widget_definitions", "widget_definitions",
			sourceFile{name: "demo.json", content: `{"definition_class":"Vendor\\Widget"}`},
			`<widget_definition definition_id="demo" definition_class="Vendor\Widget"></widget_definition>`,
		},
		{
			"widget_positions", "widget_positions",
			sourceFile{name: "demo.json", content: `{"active":false}`},
			`<widget_position position_id="demo" active="0"></widget_position>`,
		},
	} {
		t.Run(tc.directory, func(t *testing.T) {
			t.Parallel()
			root := newFixture(t)
			writeOutput(t, root, tc.directory, tc.file)
			metadataPath := "_output/" + tc.directory + "/_metadata.jsonl"
			metadata := readFile(t, root, metadataPath)
			var previous string
			for i := range 2 {
				result, err := devoutput.Export(t.Context(), root)
				if err != nil {
					t.Fatal(err)
				}
				want := devoutput.Result{Directory: filepath.Join(root, "_data"), Files: 27, Entries: 1}
				if !reflect.DeepEqual(*result, want) {
					t.Fatalf("result = %+v, want %+v", result, want)
				}
				requireXML(t, root, tc.container, "<"+tc.container+">"+tc.want+"</"+tc.container+">")
				data := readFile(t, root, "_data/"+tc.container+".xml")
				if i > 0 && data != previous {
					t.Fatal("repeated export changed XML bytes")
				}
				previous = data
			}
			if got := readFile(t, root, metadataPath); got != metadata {
				t.Fatal("export changed source metadata")
			}
			if got := readFile(t, root, "_output/"+tc.directory+"/"+tc.file.name); got != tc.file.content {
				t.Fatal("export changed source contents")
			}
		})
	}
}
