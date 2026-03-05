package billing

type Metric string

const (
	IngestTokens Metric = "ingest_tokens"
	QueryTokens  Metric = "query_tokens"
	VoiceMinutes Metric = "voice_minutes"
	ComputeGBSec Metric = "compute_gb_sec"
	StorageGB    Metric = "storage_gb"
	Events       Metric = "events"
)

var AllMetrics = []Metric{
	IngestTokens,
	QueryTokens,
	VoiceMinutes,
	ComputeGBSec,
	StorageGB,
	Events,
}

func (m Metric) Valid() bool {
	for _, valid := range AllMetrics {
		if m == valid {
			return true
		}
	}
	return false
}

func (m Metric) Unit() string {
	switch m {
	case IngestTokens, QueryTokens:
		return "tokens"
	case VoiceMinutes:
		return "minutes"
	case ComputeGBSec:
		return "gb_seconds"
	case StorageGB:
		return "gb"
	case Events:
		return "events"
	default:
		return ""
	}
}
