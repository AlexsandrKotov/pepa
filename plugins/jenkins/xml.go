package main

import (
	"fmt"
	"strings"
)

// ParamDef describes a Jenkins job parameter definition.
type ParamDef struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // StringParameterDefinition, BooleanParameterDefinition, ChoiceParameterDefinition, etc.
	Description  string   `json:"description,omitempty"`
	Default      string   `json:"default,omitempty"`
	Choices      []string `json:"choices,omitempty"`
}

// buildPipelineJobXML generates a complete Pipeline job config.xml.
// The script is embedded as a CPS (Groovy) pipeline definition.
func buildPipelineJobXML(script string, params []ParamDef, sandbox bool) string {
	var b strings.Builder
	b.WriteString(`<?xml version='1.1' encoding='UTF-8'?>`)
	b.WriteString(`<flow-definition plugin="workflow-job">`)

	// Parameters
	if len(params) > 0 {
		b.WriteString(`<properties>`)
		b.WriteString(`<hudson.model.ParametersDefinitionProperty>`)
		b.WriteString(`<parameterDefinitions>`)
		for _, p := range params {
			switch p.Type {
			case "BooleanParameterDefinition":
				b.WriteString(`<hudson.model.BooleanParameterDefinition>`)
				b.WriteString(fmt.Sprintf(`<name>%s</name>`, escapeXML(p.Name)))
				b.WriteString(fmt.Sprintf(`<description>%s</description>`, escapeXML(p.Description)))
				b.WriteString(fmt.Sprintf(`<defaultValue>%s</defaultValue>`, p.Default))
				b.WriteString(`</hudson.model.BooleanParameterDefinition>`)
			case "ChoiceParameterDefinition":
				b.WriteString(`<hudson.model.ChoiceParameterDefinition>`)
				b.WriteString(fmt.Sprintf(`<name>%s</name>`, escapeXML(p.Name)))
				b.WriteString(fmt.Sprintf(`<description>%s</description>`, escapeXML(p.Description)))
				b.WriteString(`<choices class="java.util.concurrent.CopyOnWriteArrayList">`)
				for _, c := range p.Choices {
					b.WriteString(fmt.Sprintf(`<string>%s</string>`, escapeXML(c)))
				}
				b.WriteString(`</choices>`)
				b.WriteString(`</hudson.model.ChoiceParameterDefinition>`)
			default: // StringParameterDefinition, TextParameterDefinition, PasswordParameterDefinition
				tag := "hudson.model.StringParameterDefinition"
				if p.Type == "TextParameterDefinition" {
					tag = "hudson.model.TextParameterDefinition"
				} else if p.Type == "PasswordParameterDefinition" {
					tag = "hudson.model.PasswordParameterDefinition"
				}
				b.WriteString(fmt.Sprintf(`<%s>`, tag))
				b.WriteString(fmt.Sprintf(`<name>%s</name>`, escapeXML(p.Name)))
				b.WriteString(fmt.Sprintf(`<description>%s</description>`, escapeXML(p.Description)))
				b.WriteString(fmt.Sprintf(`<defaultValue>%s</defaultValue>`, escapeXML(p.Default)))
				b.WriteString(fmt.Sprintf(`</%s>`, tag))
			}
		}
		b.WriteString(`</parameterDefinitions>`)
		b.WriteString(`</hudson.model.ParametersDefinitionProperty>`)
		b.WriteString(`</properties>`)
	} else {
		b.WriteString(`<properties/>`)
	}

	b.WriteString(`<definition class="org.jenkinsci.plugins.workflow.cps.CpsFlowDefinition" plugin="workflow-cps">`)
	b.WriteString(fmt.Sprintf(`<script>%s</script>`, escapeXML(script)))
	b.WriteString(fmt.Sprintf(`<sandbox>%v</sandbox>`, sandbox))
	b.WriteString(`</definition>`)
	b.WriteString(`<triggers/>`)
	b.WriteString(`<disabled>false</disabled>`)
	b.WriteString(`</flow-definition>`)

	return b.String()
}

// updateScriptInXML replaces only the <script> block in an existing Pipeline config.xml.
func updateScriptInXML(existingXML string, newScript string) (string, error) {
	startTag := "<script>"
	endTag := "</script>"

	startIdx := strings.Index(existingXML, startTag)
	endIdx := strings.Index(existingXML, endTag)

	if startIdx == -1 || endIdx == -1 {
		return "", fmt.Errorf("no <script> block found in config.xml")
	}
	if endIdx < startIdx {
		return "", fmt.Errorf("malformed config.xml: </script> before <script>")
	}

	var b strings.Builder
	b.WriteString(existingXML[:startIdx+len(startTag)])
	b.WriteString(escapeXML(newScript))
	b.WriteString(existingXML[endIdx:])

	return b.String(), nil
}

// escapeXML escapes special XML characters in a string.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// mapJenkinsStatus maps a Jenkins build result to a PEPA status string.
func mapJenkinsStatus(result *string, building bool) string {
	if building {
		return "running"
	}
	if result == nil {
		return "running"
	}
	switch *result {
	case "SUCCESS":
		return "success"
	case "FAILURE":
		return "failed"
	case "ABORTED":
		return "cancelled"
	case "UNSTABLE":
		return "warning"
	case "NOT_BUILT":
		return "skipped"
	default:
		return "unknown"
	}
}
