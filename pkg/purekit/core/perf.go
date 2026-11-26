package core

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pureutils/pkg/purekit/i18n"
)

// Perf aggregates operation timings. It is enabled by PUREUTILS_TRACE=1 (or legacy YNM3000_TRACE / SHOTCLIP_TRACE=1);
// the threshold is PUREUTILS_TRACE_MS / YNM3000_TRACE_MS / SHOTCLIP_TRACE_MS (default 8ms). Aggregating instead of
// logging each sample keeps pointer-move paths cheap.
type Perf struct {
	mu        sync.Mutex
	enabled   bool
	threshold time.Duration
	stats     map[string]*PerfStat
}

// PerfStat is one operation's aggregate.
type PerfStat struct {
	Count int64
	Total time.Duration
	Max   time.Duration
	Over  int64
}

var perf = NewPerf()

// NewPerf builds a Perf from the environment.
func NewPerf() *Perf {
	p := &Perf{stats: map[string]*PerfStat{}}
	traceEnv := os.Getenv("PUREUTILS_TRACE")
	if traceEnv == "" {
		traceEnv = os.Getenv("YNM3000_TRACE")
	}
	if traceEnv == "" {
		traceEnv = os.Getenv("SHOTCLIP_TRACE")
	}
	p.enabled = traceEnv == "1"
	ms := 8
	rawMS := os.Getenv("PUREUTILS_TRACE_MS")
	if rawMS == "" {
		rawMS = os.Getenv("YNM3000_TRACE_MS")
	}
	if rawMS == "" {
		rawMS = os.Getenv("SHOTCLIP_TRACE_MS")
	}
	if rawMS != "" {
		if n, err := strconv.Atoi(rawMS); err == nil && n >= 0 {
			ms = n
		}
	}
	p.threshold = time.Duration(ms) * time.Millisecond
	return p
}

// Begin returns the start time when enabled, or the zero time.
func (p *Perf) Begin() time.Time {
	if p == nil || !p.enabled {
		return time.Time{}
	}
	return time.Now()
}

// End records a sample. A zero start is a no-op.
func (p *Perf) End(name string, start time.Time) {
	if p == nil || !p.enabled || start.IsZero() {
		return
	}
	d := time.Since(start)
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.stats[name]
	if st == nil {
		st = &PerfStat{}
		p.stats[name] = st
	}
	st.Count++
	st.Total += d
	if d > st.Max {
		st.Max = d
	}
	if d >= p.threshold {
		st.Over++
	}
}

// Report renders the table sorted by total time.
func (p *Perf) Report() string {
	if p == nil || !p.enabled {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	type row struct {
		name string
		stat PerfStat
	}
	rows := make([]row, 0, len(p.stats))
	for name, st := range p.stats {
		rows = append(rows, row{name, *st})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].stat.Total > rows[j].stat.Total })
	var b strings.Builder
	b.WriteString(i18n.S("性能打点（按总耗时排序）\n", "Performance trace (sorted by total duration)\n"))
	b.WriteString(fmt.Sprintf("%-32s %8s %10s %10s %10s %8s\n",
		i18n.S("操作", "Operation"),
		i18n.S("次数", "Count"),
		i18n.S("总耗时", "Total"),
		i18n.S("平均", "Avg"),
		i18n.S("最大", "Max"),
		i18n.S("超阈值", "Over"),
	))
	for _, r := range rows {
		avg := time.Duration(0)
		if r.stat.Count > 0 {
			avg = r.stat.Total / time.Duration(r.stat.Count)
		}
		b.WriteString(fmt.Sprintf("%-32s %8d %10s %10s %10s %8d\n",
			r.name, r.stat.Count, r.stat.Total.Round(time.Millisecond),
			avg.Round(time.Microsecond), r.stat.Max.Round(time.Microsecond), r.stat.Over))
	}
	return b.String()
}

// BeginPerf starts a sample.
func BeginPerf() time.Time { return perf.Begin() }

// EndPerf ends a sample.
func EndPerf(name string, start time.Time) { perf.End(name, start) }

// PerfReport returns the current table.
func PerfReport() string { return perf.Report() }
