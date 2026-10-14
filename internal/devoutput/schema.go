package devoutput

import "strings"

// definition describes a development-output type, its record keys and sort order,
// and its release XML format.
type definition struct {
	directory string
	container string
	keys      []string
	separator string
	order     []string
	numeric   []string
	suffix    string
	ignored   []string
	element   element
}

// element describes how a record supplies an XML element's attributes, content,
// and children. A non-empty repeat names a record field whose map entries supply
// repeated children.
type element struct {
	name       string
	attributes []field
	content    field
	cdata      bool
	children   []element
	repeat     string
}

// format selects how a JSON value is represented as XML text.
type format uint8

const (
	// scalar converts a JSON scalar to text.
	scalar format = iota
	// compactJSON preserves JSON without insignificant whitespace.
	compactJSON
	// prettyJSON formats JSON with four-space indentation.
	prettyJSON
	// lines joins a JSON array of strings with newlines.
	lines
	// commas joins a JSON array of strings with commas.
	commas
)

// omission selects which values are omitted from release XML.
type omission uint8

// Omission rules:
//
//   - keep retains every value.
//   - emptyString omits JSON empty strings.
//   - emptyValue omits values recognized by isEmpty.
const (
	keep omission = iota
	emptyString
	emptyValue
)

// field maps a record value to XML text, with formatting and omission rules.
// Attribute fields also specify an XML name; forbidden prefixes reject custom
// export-handler classes.
type field struct {
	name              string
	source            string
	format            format
	omit              omission
	optional          bool
	forbiddenPrefixes []string
}

// attributes returns scalar attribute fields for whitespace-separated names.
// Each field uses the same source and XML name and omits empty strings.
func attributes(names string) []field {
	var fields []field
	for name := range strings.FieldsSeq(names) {
		fields = append(fields, field{name: name, source: name, omit: emptyString})
	}
	return fields
}

// definitions lists XenForo's built-in data types and their development-output
// and release XML formats, in export order.
var definitions = []definition{
	{
		directory: "activity_summary_definitions", container: "activity_summary_definitions",
		keys:    []string{"definition_id"},
		element: element{name: "activity_summary_definition", attributes: attributes("definition_id definition_class")},
	},
	{
		directory: "admin_navigation", container: "admin_navigation", keys: []string{"navigation_id"},
		ignored: []string{"super_admin_only"},
		element: element{name: "admin_navigation_entry", attributes: attributes("navigation_id parent_navigation_id display_order link icon admin_permission_id debug_only development_only hide_no_children")},
	},
	{
		directory: "admin_permissions", container: "admin_permission", keys: []string{"admin_permission_id"},
		element: element{name: "admin_permission", attributes: attributes("admin_permission_id display_order")},
	},
	{
		directory: "advertising_positions", container: "advertising_positions", keys: []string{"position_id"},
		element: element{name: "advertising_position", attributes: attributes("position_id active"), content: field{source: "arguments", format: compactJSON}, cdata: true},
	},
	{
		directory: "api_scopes", container: "api_scopes", keys: []string{"api_scope_id"},
		ignored: []string{"usable_with_oauth_clients"},
		element: element{name: "api_scope", attributes: attributes("api_scope_id")},
	},
	{
		directory: "bb_codes", container: "bb_codes", keys: []string{"bb_code_id"},
		element: element{
			name: "bb_code", attributes: attributes("bb_code_id bb_code_mode has_option callback_class callback_method option_regex trim_lines_after plain_children disable_smilies disable_nl2br disable_autolink allow_empty allow_signature editor_icon_type editor_icon_value active"),
			children: []element{
				{name: "replace_html", content: field{source: "replace_html"}, cdata: true},
				{name: "replace_html_email", content: field{source: "replace_html_email"}, cdata: true},
				{name: "replace_text", content: field{source: "replace_text"}, cdata: true},
			},
		},
	},
	{
		directory: "bb_code_media_sites", container: "bb_code_media_sites", keys: []string{"media_site_id"},
		element: element{
			name: "site", attributes: attributes("media_site_id site_title site_url match_is_regex match_callback_class match_callback_method embed_html_callback_class embed_html_callback_method cookie_third_parties supported active oembed_enabled oembed_api_endpoint oembed_url_scheme oembed_retain_scripts"),
			children: []element{
				{name: "match_urls", content: field{source: "match_urls"}, cdata: true},
				{name: "embed_html", content: field{source: "embed_html"}, cdata: true},
			},
		},
	},
	{
		directory: "class_extensions", container: "class_extensions", order: []string{"from_class", "to_class", "execute_order"}, numeric: []string{"execute_order"},
		element: element{
			name: "extension",
			attributes: append([]field{{
				name: "from_class", source: "from_class", omit: emptyString,
				forbiddenPrefixes: []string{`XF\AddOn\DataManager`, `XF\AddOn\DataType\`, `XF\DevelopmentOutput`},
			}}, attributes("to_class execute_order active")...),
		},
	},
	{
		directory: "code_events", container: "code_events", keys: []string{"event_id"},
		element: element{
			name: "event", content: field{source: "description"}, cdata: true,
			attributes: append(attributes("event_id"),
				field{name: "is_deprecated", source: "is_deprecated", omit: emptyString, optional: true},
				field{name: "arguments", source: "arguments", format: compactJSON, omit: emptyValue},
				field{name: "hint_description", source: "hint_description", omit: emptyValue}),
		},
	},
	{
		directory: "code_event_listeners", container: "code_event_listeners", order: []string{"event_id", "callback_class", "callback_method"},
		element: element{name: "listener", attributes: attributes("event_id execute_order callback_class callback_method active hint description")},
	},
	{
		directory: "content_type_fields", container: "content_type_fields", order: []string{"content_type", "field_name"},
		element: element{name: "field", attributes: attributes("content_type field_name"), content: field{source: "field_value"}},
	},
	{
		directory: "cron_entries", container: "cron", keys: []string{"entry_id"},
		element: element{name: "entry", attributes: attributes("entry_id cron_class cron_method active"), content: field{source: "run_rules", format: compactJSON}, cdata: true},
	},
	{
		directory: "help_pages", container: "help_pages", keys: []string{"page_id"},
		element: element{name: "help_page", attributes: attributes("page_id page_name display_order callback_class callback_method advanced_mode active")},
	},
	{
		directory: "member_stats", container: "member_stats", keys: []string{"member_stat_key"},
		element: element{name: "member_stat", attributes: attributes("member_stat_key callback_class callback_method visibility_class visibility_method sort_order sort_direction permission_limit show_value user_limit display_order overview_display active cache_lifetime"), content: field{source: "criteria", format: compactJSON}, cdata: true},
	},
	{
		directory: "navigation", container: "navigation", keys: []string{"navigation_id"},
		element: element{name: "navigation_entry", attributes: attributes("navigation_id parent_navigation_id display_order navigation_type_id enabled"), content: field{source: "type_config", format: compactJSON}, cdata: true},
	},
	{
		directory: "option_groups", container: "option_groups", keys: []string{"group_id"}, ignored: []string{"advanced"},
		element: element{name: "group", attributes: attributes("group_id icon display_order debug_only")},
	},
	{
		directory: "options", container: "options", keys: []string{"option_id"},
		element: element{
			name: "option", attributes: attributes("option_id edit_format data_type validation_class validation_method advanced"),
			children: []element{
				{name: "default_value", content: field{source: "default_value"}},
				{name: "edit_format_params", content: field{source: "edit_format_params", omit: emptyString}},
				{name: "sub_options", content: field{source: "sub_options", format: lines, omit: emptyValue}},
				{name: "relation", repeat: "relations", attributes: []field{{name: "group_id", source: "$key"}, {name: "display_order", source: "$value"}}},
			},
		},
	},
	{
		directory: "permission_interface_groups", container: "permission_interface_groups", keys: []string{"interface_group_id"},
		element: element{name: "interface_group", attributes: attributes("interface_group_id display_order is_moderator")},
	},
	{
		directory: "permissions", container: "permissions", keys: []string{"permission_group_id", "permission_id"}, separator: "-",
		element: element{name: "permission", attributes: attributes("permission_group_id permission_id permission_type depend_permission_id interface_group_id display_order")},
	},
	{
		directory: "phrases", container: "phrases", keys: []string{"title"}, suffix: ".txt",
		element: element{
			name: "phrase", content: field{source: "$text"}, cdata: true,
			attributes: append(attributes("title version_id version_string"), field{name: "global_cache", source: "global_cache", omit: emptyValue}),
		},
	},
	{
		directory: "routes", container: "routes", order: []string{"route_type", "route_prefix", "sub_name"},
		element: element{name: "route", attributes: attributes("route_type route_prefix sub_name format build_class build_method controller context action_prefix")},
	},
	{
		directory: "style_properties", container: "style_properties", keys: []string{"property_name"},
		element: element{
			name:       "style_property",
			attributes: append(attributes("property_name group_name title description property_type value_type has_variations depends_on value_group display_order"), field{name: "css_components", source: "css_components", format: commas, omit: emptyValue}),
			children: []element{
				{name: "value_parameters", content: field{source: "value_parameters", omit: emptyString}},
				{name: "value", content: field{source: "property_value", format: prettyJSON}},
			},
		},
	},
	{
		directory: "style_property_groups", container: "style_property_groups", keys: []string{"group_name"},
		element: element{name: "style_property_group", attributes: attributes("group_name title description display_order")},
	},
	{
		directory: "template_modifications", container: "template_modifications", keys: []string{"type", "modification_key"}, separator: "/", order: []string{"template", "modification_key"},
		element: element{
			name: "modification", attributes: attributes("type template modification_key description execution_order enabled action"),
			children: []element{
				{name: "find", content: field{source: "find"}, cdata: true},
				{name: "replace", content: field{source: "replace"}, cdata: true},
			},
		},
	},
	{
		directory: "templates", container: "templates", keys: []string{"type", "title"}, separator: "/", suffix: ".html",
		element: element{name: "template", attributes: attributes("type title version_id version_string"), content: field{source: "$text"}, cdata: true},
	},
	{
		directory: "widget_definitions", container: "widget_definitions", keys: []string{"definition_id"},
		element: element{name: "widget_definition", attributes: attributes("definition_id definition_class")},
	},
	{
		directory: "widget_positions", container: "widget_positions", keys: []string{"position_id"},
		element: element{name: "widget_position", attributes: attributes("position_id active")},
	},
}
