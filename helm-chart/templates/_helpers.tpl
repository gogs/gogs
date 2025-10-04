{{- define "gogs.name" -}}
gogs
{{- end -}}

{{- define "gogs.fullname" -}}
{{ printf "%s-%s" .Release.Name "gogs" }}
{{- end -}}

