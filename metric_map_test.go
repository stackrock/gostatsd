package gostatsd

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metricsFixtures() []*Metric {
	ms := []*Metric{
		{Name: "foo.bar.baz", Value: 2, Type: COUNTER, Timestamp: 10},
		{Name: "abc.def.g", Value: 3, Type: GAUGE, Timestamp: 10},
		{Name: "abc.def.g", Value: 8, Type: GAUGE, Tags: Tags{"foo:bar", "baz"}, Timestamp: 10},
		{Name: "def.g", Value: 10, Type: TIMER, Timestamp: 10},
		{Name: "def.g", Value: 1, Type: TIMER, Tags: Tags{"foo:bar", "baz"}, Timestamp: 10},
		{Name: "smp.rte", Value: 50, Type: COUNTER, Timestamp: 10},
		{Name: "smp.rte", Value: 50, Type: COUNTER, Tags: Tags{"foo:bar", "baz"}, Timestamp: 10},
		{Name: "smp.rte", Value: 5, Type: COUNTER, Tags: Tags{"foo:bar", "baz"}, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "joe", Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "joe", Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "bob", Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "john", Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "john", Type: SET, Tags: Tags{"foo:bar", "baz"}, Timestamp: 10},
		{Name: "timer_sampling", Value: 10, Type: TIMER, Rate: 0.1, Timestamp: 10},
		{Name: "timer_sampling", Value: 30, Type: TIMER, Rate: 0.1, Timestamp: 10},
		{Name: "timer_sampling", Value: 50, Type: TIMER, Rate: 0.1, Timestamp: 10},
		{Name: "counter_sampling", Value: 2, Type: COUNTER, Rate: 0.25, Timestamp: 10},
		{Name: "counter_sampling", Value: 5, Type: COUNTER, Rate: 0.25, Timestamp: 10},
	}
	for i, m := range ms {
		if ms[i].Rate == 0.0 {
			ms[i].Rate = 1.0
		}
		ms[i].TagsKey = m.FormatTagsKey()
	}
	return ms
}

func TestReceive(t *testing.T) {
	t.Parallel()
	assrt := assert.New(t)

	mm := NewMetricMap(false)

	tests := metricsFixtures()
	for _, metric := range tests {
		mm.Receive(metric)
	}

	expectedCounters := Counters{
		"foo.bar.baz": map[string]Counter{
			"": {Value: 2, Timestamp: 10},
		},
		"smp.rte": map[string]Counter{
			"":            {Value: 50, Timestamp: 10},
			"baz,foo:bar": {Value: 55, Timestamp: 10, Tags: Tags{"baz", "foo:bar"}},
		},
		"counter_sampling": map[string]Counter{
			"": {Value: 28, Timestamp: 10},
		},
	}
	assrt.Equal(expectedCounters, mm.Counters)

	expectedGauges := Gauges{
		"abc.def.g": map[string]Gauge{
			"":            {Value: 3, Timestamp: 10},
			"baz,foo:bar": {Value: 8, Timestamp: 10, Tags: Tags{"baz", "foo:bar"}},
		},
	}
	assrt.Equal(expectedGauges, mm.Gauges)

	expectedTimers := Timers{
		"def.g": map[string]Timer{
			"":            {Values: []float64{10}, Timestamp: 10, SampledCount: 1},
			"baz,foo:bar": {Values: []float64{1}, Timestamp: 10, SampledCount: 1, Tags: Tags{"baz", "foo:bar"}},
		},
		"timer_sampling": map[string]Timer{
			"": {Values: []float64{10, 30, 50}, Timestamp: 10, SampledCount: 30},
		},
	}
	assrt.Equal(expectedTimers, mm.Timers)

	expectedSets := Sets{
		"uniq.usr": map[string]Set{
			"": {
				Values: map[string]struct{}{
					"joe":  {},
					"bob":  {},
					"john": {},
				},
				Timestamp: 10,
			},
			"baz,foo:bar": {
				Values: map[string]struct{}{
					"john": {},
				},
				Timestamp: 10,
				Tags:      Tags{"baz", "foo:bar"},
			},
		},
	}
	assrt.Equal(expectedSets, mm.Sets)
}

func benchmarkReceive(metric Metric, b *testing.B) {
	ma := NewMetricMap(false)
	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		ma.Receive(&metric)
	}
}

func BenchmarkReceiveCounter(b *testing.B) {
	benchmarkReceive(Metric{Name: "foo.bar.baz", Value: 2, Type: COUNTER}, b)
}

func BenchmarkReceiveGauge(b *testing.B) {
	benchmarkReceive(Metric{Name: "abc.def.g", Value: 3, Type: GAUGE}, b)
}

func BenchmarkReceiveTimer(b *testing.B) {
	benchmarkReceive(Metric{Name: "def.g", Value: 10, Type: TIMER}, b)
}

func BenchmarkReceiveSet(b *testing.B) {
	benchmarkReceive(Metric{Name: "uniq.usr", StringValue: "joe", Type: SET}, b)
}

func BenchmarkReceives(b *testing.B) {
	ma := NewMetricMap(false)
	tests := metricsFixtures()
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, metric := range tests {
			ma.Receive(metric)
		}
	}
}

func TestMetricMapDispatch(t *testing.T) {
	ctx, done := testContext(t)
	defer done()

	mm := NewMetricMap(false)
	metrics := metricsFixtures()
	for _, metric := range metrics {
		mm.Receive(metric)
	}
	ch := &capturingHandler{}

	mm.DispatchMetrics(ctx, ch)

	expected := []*Metric{
		{Name: "abc.def.g", Value: 3, Rate: 1, Type: GAUGE, Timestamp: 10},
		{Name: "abc.def.g", Value: 8, Rate: 1, TagsKey: "baz,foo:bar", Tags: Tags{"baz", "foo:bar"}, Type: GAUGE, Timestamp: 10},
		{Name: "counter_sampling", Value: (2 + 5) / 0.25, Rate: 1, Type: COUNTER, Timestamp: 10},
		{Name: "def.g", Value: 10, Rate: 1, Type: TIMER, Timestamp: 10},
		{Name: "def.g", Value: 1, Rate: 1, TagsKey: "baz,foo:bar", Tags: Tags{"baz", "foo:bar"}, Type: TIMER, Timestamp: 10},
		{Name: "foo.bar.baz", Value: 2, Rate: 1, Type: COUNTER, Timestamp: 10},
		{Name: "smp.rte", Value: 50, Rate: 1, Type: COUNTER, Timestamp: 10},
		{Name: "smp.rte", Value: 50 + 5, Rate: 1, TagsKey: "baz,foo:bar", Tags: Tags{"baz", "foo:bar"}, Type: COUNTER, Timestamp: 10},
		{Name: "timer_sampling", Value: 10, Rate: 0.1, Type: TIMER, Timestamp: 10},
		{Name: "timer_sampling", Value: 30, Rate: 0.1, Type: TIMER, Timestamp: 10},
		{Name: "timer_sampling", Value: 50, Rate: 0.1, Type: TIMER, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "bob", Rate: 1, Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "joe", Rate: 1, Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "john", Rate: 1, Type: SET, Timestamp: 10},
		{Name: "uniq.usr", StringValue: "john", Rate: 1, TagsKey: "baz,foo:bar", Tags: Tags{"baz", "foo:bar"}, Type: SET, Timestamp: 10},
	}

	cmpSort := func(slice []*Metric) func(i, j int) bool {
		return func(i, j int) bool {
			if slice[i].Name == slice[j].Name {
				if len(slice[i].Tags) == len(slice[j].Tags) { // This is not exactly accurate, but close enough with our data
					if slice[i].Type == SET {
						return slice[i].StringValue < slice[j].StringValue
					} else {
						return slice[i].Value < slice[j].Value
					}
				}
				return len(slice[i].Tags) < len(slice[j].Tags)
			}
			return slice[i].Name < slice[j].Name
		}
	}

	actual := ch.GetMetrics()

	sort.Slice(actual, cmpSort(actual))
	sort.Slice(expected, cmpSort(expected))

	require.EqualValues(t, expected, actual)
}

func TestMetricMapMerge(t *testing.T) {
	metrics1 := []*Metric{
		{Name: "TestMetricMapMerge.counter", Value: 10, Rate: 1, Type: COUNTER, Timestamp: 10},
		{Name: "TestMetricMapMerge.gauge", Value: 10, Type: GAUGE, Timestamp: 10},
		{Name: "TestMetricMapMerge.timer", Value: 10, Rate: 1, Type: TIMER, Timestamp: 10},
		{Name: "TestMetricMapMerge.timer", Value: 10, Rate: 0.1, Type: TIMER, Timestamp: 10},
		{Name: "TestMetricMapMerge.set", StringValue: "abc", Type: SET, Timestamp: 10},
	}
	metrics2 := []*Metric{
		{Name: "TestMetricMapMerge.counter", Value: 20, Rate: 0.1, Type: COUNTER, Timestamp: 20},
		{Name: "TestMetricMapMerge.gauge", Value: 20, Type: GAUGE, Timestamp: 20},
		{Name: "TestMetricMapMerge.timer", Value: 20, Rate: 1, Type: TIMER, Timestamp: 20},
		{Name: "TestMetricMapMerge.timer", Value: 20, Rate: 0.1, Type: TIMER, Timestamp: 20},
		{Name: "TestMetricMapMerge.set", StringValue: "def", Type: SET, Timestamp: 20},
	}

	m1 := NewMetricMap(false)
	for _, m := range metrics1 {
		m1.Receive(m)
	}

	m2 := NewMetricMap(false)
	for _, m := range metrics2 {
		m2.Receive(m)
	}

	merged := NewMetricMap(false)
	merged.Merge(m1)
	merged.Merge(m2)

	expected := NewMetricMap(false)
	expected.Counters = Counters{
		"TestMetricMapMerge.counter": map[string]Counter{
			"": {
				Value:     10 + (20 / 0.1),
				Timestamp: 20,
			},
		},
	}
	expected.Timers = Timers{
		"TestMetricMapMerge.timer": map[string]Timer{
			"": {
				SampledCount: 1 + (1 / 0.1) + 1 + (1 / 0.1),
				Values:       []float64{10, 10, 20, 20},
				Timestamp:    20,
			},
		},
	}
	expected.Gauges = Gauges{
		"TestMetricMapMerge.gauge": map[string]Gauge{
			"": {
				Value:     20, // most recent value wins
				Timestamp: 20,
			},
		},
	}
	expected.Sets = Sets{
		"TestMetricMapMerge.set": map[string]Set{
			"": {
				Values:    map[string]struct{}{"abc": {}, "def": {}},
				Timestamp: 20,
			},
		},
	}
	require.Equal(t, expected.Counters, merged.Counters)
	require.Equal(t, expected.Timers, merged.Timers)
	require.Equal(t, expected.Gauges, merged.Gauges)
	require.Equal(t, expected.Sets, merged.Sets)
}

func TestMetricMapSplit(t *testing.T) {
	mmOriginal := NewMetricMap(false)
	mmOriginal.Counters["m"] = map[string]Counter{
		"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Value: 10},
		"t.s.h2": {Tags: Tags{"t"}, Hostname: "h2", Value: 20},
		"t.s.h3": {Tags: Tags{"t"}, Hostname: "h3", Value: 30},
		"t.s.h4": {Tags: Tags{"t"}, Hostname: "h4", Value: 40},
		"t.s.h5": {Tags: Tags{"t"}, Hostname: "h5", Value: 50},
	}
	mmOriginal.Gauges["m"] = map[string]Gauge{
		"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Value: 10},
		"t.s.h2": {Tags: Tags{"t"}, Hostname: "h2", Value: 20},
		"t.s.h3": {Tags: Tags{"t"}, Hostname: "h3", Value: 30},
		"t.s.h4": {Tags: Tags{"t"}, Hostname: "h4", Value: 40},
		"t.s.h5": {Tags: Tags{"t"}, Hostname: "h5", Value: 50},
	}
	mmOriginal.Timers["m"] = map[string]Timer{
		"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Values: []float64{10, 50}},
		"t.s.h2": {Tags: Tags{"t"}, Hostname: "h2", Values: []float64{20, 40}},
		"t.s.h3": {Tags: Tags{"t"}, Hostname: "h3", Values: []float64{30, 30}},
		"t.s.h4": {Tags: Tags{"t"}, Hostname: "h4", Values: []float64{40, 20}},
		"t.s.h5": {Tags: Tags{"t"}, Hostname: "h5", Values: []float64{50, 10}},
	}
	mmOriginal.Sets["m"] = map[string]Set{
		"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Values: map[string]struct{}{"10": {}, "50": {}}},
		"t.s.h2": {Tags: Tags{"t"}, Hostname: "h2", Values: map[string]struct{}{"20": {}, "40": {}}},
		"t.s.h3": {Tags: Tags{"t"}, Hostname: "h3", Values: map[string]struct{}{"30": {}, "3.0": {}}},
		"t.s.h4": {Tags: Tags{"t"}, Hostname: "h4", Values: map[string]struct{}{"40": {}, "20": {}}},
		"t.s.h5": {Tags: Tags{"t"}, Hostname: "h5", Values: map[string]struct{}{"50": {}, "10": {}}},
	}

	mmMerged := NewMetricMap(false)
	mms := mmOriginal.Split(2)
	for _, mmSplit := range mms {
		// Make sure something landed in each (don't use mmSplit.IsEmpty() to ensure that all types are split)
		require.True(t, len(mmSplit.Counters) > 0)
		require.True(t, len(mmSplit.Gauges) > 0)
		require.True(t, len(mmSplit.Timers) > 0)
		require.True(t, len(mmSplit.Sets) > 0)
		mmMerged.Merge(mmSplit)
	}
	// Make sure when merge back they are the same
	require.EqualValues(t, mmOriginal, mmMerged)
}

func TestMetricMapIsEmpty(t *testing.T) {
	mm := NewMetricMap(false)
	require.True(t, mm.IsEmpty())

	// Counter
	mm.Counters["m"] = map[string]Counter{"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Value: 10}}
	require.False(t, mm.IsEmpty())
	mm.Counters.Delete("m")
	require.True(t, mm.IsEmpty())

	// Gauge
	mm.Gauges["m"] = map[string]Gauge{"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Value: 10}}
	require.False(t, mm.IsEmpty())
	mm.Gauges.Delete("m")
	require.True(t, mm.IsEmpty())

	// Timer
	mm.Timers["m"] = map[string]Timer{"t.s.h1": {Tags: Tags{"t"}, Hostname: "h1", Values: []float64{10}}}
	require.False(t, mm.IsEmpty())
	mm.Timers.Delete("m")
	require.True(t, mm.IsEmpty())

	// Set
	mm.Sets["m"] = map[string]Set{"t.s.h1": {Tags: Tags{"t"}, Hostname: "h5", Values: map[string]struct{}{"10": {}}}}
	require.False(t, mm.IsEmpty())
	mm.Sets.Delete("m")
	require.True(t, mm.IsEmpty())
}
