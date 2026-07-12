package routing

import (
	"math"
	"net/netip"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/platform"
)

type scoreTestPool struct {
	entries map[node.Hash]*node.NodeEntry
}

func (p *scoreTestPool) GetEntry(h node.Hash) (*node.NodeEntry, bool) {
	e, ok := p.entries[h]
	return e, ok
}

func (p *scoreTestPool) GetPlatform(string) (*platform.Platform, bool) { return nil, false }
func (p *scoreTestPool) GetPlatformByName(string) (*platform.Platform, bool) {
	return nil, false
}
func (p *scoreTestPool) RangePlatforms(func(*platform.Platform) bool) {}

func TestCalculateScore_BalancedPrefersIdleWhenLatencyClose(t *testing.T) {
	hIdle := node.HashFromRawOptions([]byte(`{"id":"idle"}`))
	hBusy := node.HashFromRawOptions([]byte(`{"id":"busy"}`))

	pool := &scoreTestPool{entries: map[node.Hash]*node.NodeEntry{
		hIdle: node.NewNodeEntry(hIdle, nil, time.Now(), 16),
		hBusy: node.NewNodeEntry(hBusy, nil, time.Now(), 16),
	}}
	ipIdle := netip.MustParseAddr("1.1.1.1")
	ipBusy := netip.MustParseAddr("2.2.2.2")
	pool.entries[hIdle].SetEgressIP(ipIdle)
	pool.entries[hBusy].SetEgressIP(ipBusy)

	plat := platform.NewPlatform("p1", "P1", nil, nil)
	plat.AllocationPolicy = platform.AllocationPolicyBalanced

	// idle: 0 租约，busy: 3 租约
	stats := NewIPLoadStats()
	for i := 0; i < 3; i++ {
		stats.Inc(ipBusy)
	}

	latIdle := 100 * time.Millisecond
	latBusy := 100 * time.Millisecond
	sIdle := calculateScore(hIdle, latIdle, plat, stats, pool)
	sBusy := calculateScore(hBusy, latBusy, plat, stats, pool)
	if !(sIdle < sBusy) {
		t.Fatalf("same latency: idle score=%v should be lower than busy=%v", sIdle, sBusy)
	}

	// 150ms 空闲 vs 80ms 已有 2 租约：空闲权重下仍应选空闲
	stats2 := NewIPLoadStats()
	stats2.Inc(ipBusy)
	stats2.Inc(ipBusy)
	sHighIdle := calculateScore(hIdle, 150*time.Millisecond, plat, stats2, pool)
	sLowBusy := calculateScore(hBusy, 80*time.Millisecond, plat, stats2, pool)
	if !(sHighIdle < sLowBusy) {
		t.Fatalf("idle-heavy: 150ms/0lease score=%v should beat 80ms/2lease score=%v", sHighIdle, sLowBusy)
	}

	// 公式核对：Score = (C+1)^1.5 * max(ms,1)
	wantBusy := math.Pow(4, balancedIdleExponent) * 100
	if math.Abs(sBusy-wantBusy) > 1e-6 {
		t.Fatalf("busy score got %v want %v", sBusy, wantBusy)
	}
	wantIdle := math.Pow(1, balancedIdleExponent) * 100
	if math.Abs(sIdle-wantIdle) > 1e-6 {
		t.Fatalf("idle score got %v want %v", sIdle, wantIdle)
	}
}

func TestCalculateScore_BalancedEmptyLatencyUsesIdleFactor(t *testing.T) {
	h := node.HashFromRawOptions([]byte(`{"id":"x"}`))
	pool := &scoreTestPool{entries: map[node.Hash]*node.NodeEntry{
		h: node.NewNodeEntry(h, nil, time.Now(), 16),
	}}
	ip := netip.MustParseAddr("9.9.9.9")
	pool.entries[h].SetEgressIP(ip)
	plat := platform.NewPlatform("p1", "P1", nil, nil)
	plat.AllocationPolicy = platform.AllocationPolicyBalanced
	stats := NewIPLoadStats()
	stats.Inc(ip)
	stats.Inc(ip)

	got := calculateScore(h, 0, plat, stats, pool)
	want := math.Pow(3, balancedIdleExponent)
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("empty latency score got %v want %v", got, want)
	}
}
