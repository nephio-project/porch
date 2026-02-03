{{- define "gvList" -}}
{{- $groupVersions := . -}}

---
title: "API Reference"
type: docs
weight: 2
description: API reference documentation for Porch resources
---

## Packages
{{- range $groupVersions }}
- {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
