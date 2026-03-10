{{/*
Expand the name of the chart.
*/}}
{{- define "adhive.name" -}}
{{- default .Chart.Name .Values.global.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "adhive.fullname" -}}
{{- if .Values.global.fullnameOverride }}
{{- .Values.global.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.global.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "adhive.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "adhive.labels" -}}
helm.sh/chart: {{ include "adhive.chart" . }}
{{ include "adhive.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "adhive.selectorLabels" -}}
app.kubernetes.io/name: {{ include "adhive.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "adhive.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "adhive.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create PVC names
*/}}
{{- define "adhive.databasePVC" -}}
{{- printf "%s-database" (include "adhive.fullname" .) }}
{{- end }}

{{- define "adhive.archivesPVC" -}}
{{- printf "%s-archives" (include "adhive.fullname" .) }}
{{- end }}

{{- define "adhive.thumbnailsPVC" -}}
{{- printf "%s-thumbnails" (include "adhive.fullname" .) }}
{{- end }}
