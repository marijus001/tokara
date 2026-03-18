package compress

import "testing"

func TestClassifyBlank(t *testing.T) {
	if got := ClassifyLine(""); got != CatBlank {
		t.Errorf("expected blank, got %s", got)
	}
	if got := ClassifyLine("   "); got != CatBlank {
		t.Errorf("expected blank for whitespace, got %s", got)
	}
}

func TestClassifyDebug(t *testing.T) {
	cases := []string{
		"console.log('test')",
		"  debugger",
		"logger.info('msg')",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatDebug {
			t.Errorf("ClassifyLine(%q) = %s, want debug", c, got)
		}
	}
}

func TestClassifyComment(t *testing.T) {
	cases := []string{
		"// this is a comment",
		"/* block comment */",
		"# python comment",
		" * jsdoc line",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatComment {
			t.Errorf("ClassifyLine(%q) = %s, want comment", c, got)
		}
	}
}

func TestClassifySignature(t *testing.T) {
	cases := []string{
		"function validateToken(t) {",
		"async function fetchData(url) {",
		"class UserService {",
		"export default function App() {",
		"def process_request(req):",
		"func main() {",
		"pub fn new(config: Config) -> Self {",
		"const handler = (req, res) =>",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatSignature {
			t.Errorf("ClassifyLine(%q) = %s, want signature", c, got)
		}
	}
}

func TestClassifyImport(t *testing.T) {
	cases := []string{
		"import React from 'react'",
		"from flask import Flask",
		"export { default } from './module'",
		"require('express')",
		"using System.Collections.Generic;",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatImport {
			t.Errorf("ClassifyLine(%q) = %s, want import", c, got)
		}
	}
}

func TestClassifyType(t *testing.T) {
	cases := []string{
		"type Config struct {",
		"interface UserProps {",
		"enum Status {",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatType {
			t.Errorf("ClassifyLine(%q) = %s, want type", c, got)
		}
	}
}

func TestClassifyStructural(t *testing.T) {
	cases := []string{
		"## Section Header",
		"---",
		"@media (max-width: 768px) {",
		".container {",
		"[database]",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatStructural {
			t.Errorf("ClassifyLine(%q) = %s, want structural", c, got)
		}
	}
}

func TestClassifyFence(t *testing.T) {
	if got := ClassifyLine("```javascript"); got != CatFence {
		t.Errorf("expected fence, got %s", got)
	}
	if got := ClassifyLine("```"); got != CatFence {
		t.Errorf("expected fence, got %s", got)
	}
}

func TestClassifyKeyBody(t *testing.T) {
	cases := []string{
		"return result",
		"this.name = name",
		"  yield value",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatKeyBody {
			t.Errorf("ClassifyLine(%q) = %s, want key_body", c, got)
		}
	}
}

func TestClassifyBody(t *testing.T) {
	cases := []string{
		"x = x + 1",
		"doSomething()",
		"  arr.push(item)",
	}
	for _, c := range cases {
		if got := ClassifyLine(c); got != CatBody {
			t.Errorf("ClassifyLine(%q) = %s, want body", c, got)
		}
	}
}

func TestIsAnchor(t *testing.T) {
	anchors := []string{CatSignature, CatImport, CatType, CatStructural, CatFence}
	for _, a := range anchors {
		if !IsAnchor(a) {
			t.Errorf("expected %s to be anchor", a)
		}
	}
	nonAnchors := []string{CatBlank, CatDebug, CatComment, CatBody, CatKeyBody}
	for _, na := range nonAnchors {
		if IsAnchor(na) {
			t.Errorf("expected %s to NOT be anchor", na)
		}
	}
}
