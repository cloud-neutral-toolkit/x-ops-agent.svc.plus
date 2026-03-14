package analyzer

import (
	"sort"
	"strings"
	"time"

	"github.com/yourname/XOpsAgent/internal/model"
	"github.com/yourname/XOpsAgent/utils"
)

// AnalyzeAbnormalServices identifies services with abnormal metrics.
func AnalyzeAbnormalServices(metrics []model.Metric) []string {
	type score struct {
		name  string
		value float64
	}

	scores := make(map[string]float64)
	for _, metric := range metrics {
		if metric.ServiceName == "" {
			continue
		}
		if metric.ErrorRate >= 0.05 {
			scores[metric.ServiceName] += 2
		}
		if metric.LatencyAvg >= 500 {
			scores[metric.ServiceName]++
		}
		if metric.LatencyMax >= 1000 {
			scores[metric.ServiceName]++
		}
	}

	ordered := make([]score, 0, len(scores))
	for name, value := range scores {
		if value <= 0 {
			continue
		}
		ordered = append(ordered, score{name: name, value: value})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].value == ordered[j].value {
			return ordered[i].name < ordered[j].name
		}
		return ordered[i].value > ordered[j].value
	})

	services := make([]string, 0, len(ordered))
	for _, item := range ordered {
		services = append(services, item.name)
	}
	return services
}

// DetectTimeRange determines the time window for abnormal behavior.
func DetectTimeRange(metrics []model.Metric, services []string) utils.TimeRange {
	if len(metrics) == 0 {
		return utils.TimeRange{}
	}
	selected := make(map[string]struct{}, len(services))
	for _, service := range services {
		selected[service] = struct{}{}
	}

	var start time.Time
	var end time.Time
	var found bool
	for _, metric := range metrics {
		if len(selected) > 0 {
			if _, ok := selected[metric.ServiceName]; !ok {
				continue
			}
		}
		if !found || metric.Timestamp.Before(start) {
			start = metric.Timestamp
		}
		if !found || metric.Timestamp.After(end) {
			end = metric.Timestamp
		}
		found = true
	}
	if !found {
		return utils.TimeRange{}
	}
	return utils.TimeRange{Start: start, End: end}
}

// FindErrorTraces filters traces to those containing errors.
func FindErrorTraces(traces []model.Trace) []model.Trace {
	out := make([]model.Trace, 0, len(traces))
	for _, trace := range traces {
		for _, span := range trace.Spans {
			if span.Error {
				out = append(out, trace)
				break
			}
			if stringsEqualFold(span.Tags["error"], "true") {
				out = append(out, trace)
				break
			}
		}
	}
	return out
}

// LocateRootSpan locates the span suspected to cause the error.
func LocateRootSpan(trace model.Trace) model.Span {
	var root model.Span
	var candidate model.Span
	var foundRoot bool
	var foundCandidate bool
	for _, span := range trace.Spans {
		if span.ParentID == "" && !foundRoot {
			root = span
			foundRoot = true
		}
		if !span.Error && !stringsEqualFold(span.Tags["error"], "true") {
			continue
		}
		if !foundCandidate || span.DurationMs > candidate.DurationMs {
			candidate = span
			foundCandidate = true
		}
	}
	if foundCandidate {
		return candidate
	}
	if foundRoot {
		return root
	}
	if len(trace.Spans) > 0 {
		return trace.Spans[0]
	}
	return model.Span{}
}

// ResolveLocation finds pod and node info for a span.
func ResolveLocation(span model.Span) (string, string) {
	pod := span.Tags["k8s.pod.name"]
	if pod == "" {
		pod = span.Tags["pod"]
	}
	node := span.Tags["k8s.node.name"]
	if node == "" {
		node = span.Tags["node"]
	}
	return pod, node
}

// SuggestAction suggests actions based on root cause.
func SuggestAction(cause model.RootCause) model.ActionSuggestion {
	switch cause.Type {
	case "OOM":
		return model.ActionSuggestion{
			Suggestions: []string{
				"Inspect recent pod restarts and confirm the container was OOMKilled.",
				"Increase memory requests and limits for the affected workload before restarting it.",
				"Review the latest rollout or traffic spike that increased memory pressure.",
			},
		}
	case "TCP Reset":
		return model.ActionSuggestion{
			Suggestions: []string{
				"Check upstream or downstream connection resets around the failure window.",
				"Validate service mesh, load balancer, and keepalive timeout settings.",
				"Roll back recent network or proxy configuration changes if the resets started after a deploy.",
			},
		}
	default:
		return model.ActionSuggestion{
			Suggestions: []string{
				"Correlate the failure window with recent deploys, config changes, and dependency incidents.",
				"Collect logs and traces for the suspect service before taking remediation actions.",
				"Prepare a safe rollback plan if the issue keeps reproducing in production.",
			},
		}
	}
}

func stringsEqualFold(value, target string) bool {
	return len(value) > 0 && len(target) > 0 && strings.EqualFold(value, target)
}
