{{- define "gatus-config-controller.fullname" -}}
{{- printf "%s" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "gatus-config-controller.labels" -}}
app.kubernetes.io/name: gatus-config-controller
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "gatus-config-controller.selectorLabels" -}}
app.kubernetes.io/name: gatus-config-controller
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
