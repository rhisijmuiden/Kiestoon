package freeswitch

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// node is an XML element.
type node struct {
	Name     string
	Attr     map[string]string
	Children []*node
}

func (n *node) attr(k string) string { return n.Attr[k] }

// all returns the descendants with the element name.
func (n *node) all(name string) []*node {
	var out []*node
	var walk func(x *node)
	walk = func(x *node) {
		for _, c := range x.Children {
			if c.Name == name {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// kids returns the direct children with the element name.
func (n *node) kids(name string) []*node {
	var out []*node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// params maps <param name= value=> (and <variable>) below n.
func (n *node) params(elem string) map[string]string {
	out := map[string]string{}
	for _, p := range n.all(elem) {
		out[strings.ToLower(p.attr("name"))] = p.attr("value")
	}
	return out
}

// parseXML reads a FreeSWITCH configuration file into a tree. FreeSWITCH files are often not
// strict XML (several roots, X-PRE-PROCESS lines); they are wrapped in one root first.
func parseXML(data []byte) *node {
	root := &node{Name: "#root", Attr: map[string]string{}}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if i := bytes.Index(data, []byte("?>")); i >= 0 && bytes.HasPrefix(bytes.TrimSpace(data), []byte("<?xml")) {
		data = data[i+2:]
	}
	dec := xml.NewDecoder(io.MultiReader(strings.NewReader("<kt-root>"), bytes.NewReader(data), strings.NewReader("</kt-root>")))
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity
	stack := []*node{root}
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{Name: t.Name.Local, Attr: map[string]string{}}
			for _, a := range t.Attr {
				n.Attr[a.Name.Local] = a.Value
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return root
}

var reVar = regexp.MustCompile(`\$\$\{([A-Za-z0-9_]+)\}`)

// expand replaces $${var} with the values from vars.xml.
func expand(s string, vars map[string]string) string {
	return reVar.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vars[reVar.FindStringSubmatch(m)[1]]; ok {
			return v
		}
		return m
	})
}

var rePreSet = regexp.MustCompile(`(?i)<X-PRE-PROCESS\s+cmd="set"\s+data="([^=]+)=([^"]*)"`)

// readVars reads the X-PRE-PROCESS set lines of vars.xml.
func readVars(data []byte) map[string]string {
	out := map[string]string{}
	for _, m := range rePreSet.FindAllSubmatch(data, -1) {
		out[strings.TrimSpace(string(m[1]))] = string(m[2])
	}
	return out
}
