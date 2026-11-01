package devoutput

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"maps"
	"slices"
)

// xmlElement is an XML node with ordered attributes and children.
// Its content is encoded as character data or CDATA.
type xmlElement struct {
	name       string
	attributes []xml.Attr
	content    string
	cdata      bool
	children   []xmlElement
}

// convert applies rule to r, validating XML text and expanding child elements.
// It returns nil if the content field's omission rule omits the element.
func (rule element) convert(r record) (*xmlElement, error) {
	node := &xmlElement{name: rule.name, cdata: rule.cdata}
	node.attributes = make([]xml.Attr, 0, len(rule.attributes))
	for _, attr := range rule.attributes {
		value, omitted, err := attr.value(r)
		if err != nil {
			return nil, err
		}
		if omitted {
			continue
		}
		if err := validateText(value); err != nil {
			return nil, fmt.Errorf("attribute %s: %w", attr.name, err)
		}
		node.attributes = append(node.attributes, xml.Attr{Name: xml.Name{Local: attr.name}, Value: value})
	}
	if rule.content.source != "" {
		value, omitted, err := rule.content.value(r)
		if err != nil {
			return nil, err
		}
		if omitted {
			return nil, nil
		}
		if err := validateText(value); err != nil {
			return nil, fmt.Errorf("element %s: %w", rule.name, err)
		}
		node.content = value
	}
	for _, childRule := range rule.children {
		rows, err := childRows(childRule, r)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			child, err := childRule.convert(row)
			if err != nil {
				return nil, err
			}
			if child != nil {
				node.children = append(node.children, *child)
			}
		}
	}
	return node, nil
}

// childRows returns the records childRule is converted against: r itself, or
// one record per entry of its repeated collection, in key order.
func childRows(childRule element, r record) ([]record, error) {
	if childRule.repeat == "" {
		return []record{r}, nil
	}
	raw, ok := r[childRule.repeat]
	if !ok {
		return nil, fmt.Errorf("missing field %q", childRule.repeat)
	}
	var values record
	if !isEmpty(raw) {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("field %q: %w", childRule.repeat, err)
		}
	}
	var rows []record
	for _, name := range slices.Sorted(maps.Keys(values)) {
		key, _ := json.Marshal(name)
		rows = append(rows, record{"$key": key, "$value": values[name]})
	}
	return rows, nil
}

// MarshalXML encodes node using its name, attributes, content, and children,
// implementing [xml.Marshaler].
func (node xmlElement) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: node.name}
	start.Attr = node.attributes
	value := struct {
		Content  string       `xml:",chardata"`
		CDATA    string       `xml:",cdata"`
		Children []xmlElement `xml:",any"`
	}{Children: node.children}
	if node.cdata {
		value.CDATA = node.content
	} else {
		value.Content = node.content
	}
	return e.EncodeElement(value, start)
}

// render encodes nodes beneath container as an indented XML document with a
// declaration and trailing newline.
func render(container string, nodes []xmlElement) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	e := xml.NewEncoder(&out)
	e.Indent("", "  ")
	if err := e.Encode(xmlElement{name: container, children: nodes}); err != nil {
		return nil, err
	}
	if err := e.Close(); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
