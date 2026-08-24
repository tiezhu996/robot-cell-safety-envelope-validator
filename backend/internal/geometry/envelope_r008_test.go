package geometry

import (
	"testing"

	"robot-cell-safety-envelope-validator/backend/internal/dto"
)

func zoneBlock(id uint, name, zoneType string, speed float64) ZoneVolume {
	return ZoneVolume{
		ID: id, Name: name, ZoneType: zoneType,
		Polygon:       Polygon{Ring: []Point2D{{X: -50, Y: -50}, {X: 50, Y: -50}, {X: 50, Y: 50}, {X: -50, Y: 50}, {X: -50, Y: -50}}},
		MinHeightMM:   0, MaxHeightMM: 1000, SpeedLimitMMS: speed,
	}
}

func TestEvaluateEnvelopeShortTrajectoryNoPanic(t *testing.T) {
	points := []dto.TrajectoryPoint{{XMM: 0, YMM: 0, ZMM: 0, TimeMS: 0, SpeedMMS: 10}}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("EvaluateEnvelope panicked on a short trajectory: %v", recovered)
		}
	}()
	events := EvaluateEnvelope(points, 50, []ZoneVolume{zoneBlock(1, "z", "restricted", 0)})
	if len(events) != 0 {
		t.Fatalf("single-point trajectory must not produce collision events: %+v", events)
	}
}

func TestEvaluateEnvelopeDetectsSegmentCollisions(t *testing.T) {
	points := []dto.TrajectoryPoint{
		{XMM: -1000, YMM: 0, ZMM: 500, TimeMS: 0, SpeedMMS: 100},
		{XMM: 0, YMM: 0, ZMM: 500, TimeMS: 1000, SpeedMMS: 100},
		{XMM: 1000, YMM: 0, ZMM: 500, TimeMS: 2000, SpeedMMS: 100},
	}
	events := EvaluateEnvelope(points, 50, []ZoneVolume{zoneBlock(1, "restricted block", "restricted", 0)})
	for _, event := range events {
		if event.ZoneID == 1 && event.Violation {
			return
		}
	}
	t.Fatalf("expected a violation for the restricted-zone segment, got %+v", events)
}

func TestEvaluateEnvelopeResultIsolation(t *testing.T) {
	zones := []ZoneVolume{
		{
			ID: 1, Name: "block-one", ZoneType: "restricted",
			Polygon:       Polygon{Ring: []Point2D{{X: -50, Y: -50}, {X: 50, Y: -50}, {X: 50, Y: 50}, {X: -50, Y: 50}, {X: -50, Y: -50}}},
			MinHeightMM:   0, MaxHeightMM: 1000, SpeedLimitMMS: 0,
		},
		{
			ID: 2, Name: "block-two", ZoneType: "restricted",
			Polygon:       Polygon{Ring: []Point2D{{X: -50, Y: 2950}, {X: 50, Y: 2950}, {X: 50, Y: 3050}, {X: -50, Y: 3050}, {X: -50, Y: 2950}}},
			MinHeightMM:   0, MaxHeightMM: 1000, SpeedLimitMMS: 0,
		},
	}
	first := EvaluateEnvelope([]dto.TrajectoryPoint{
		{XMM: -1000, YMM: 0, ZMM: 500, TimeMS: 0, SpeedMMS: 100},
		{XMM: 1000, YMM: 0, ZMM: 500, TimeMS: 1000, SpeedMMS: 100},
	}, 50, zones)
	firstLen := len(first)
	if firstLen == 0 {
		t.Fatal("first evaluation produced no events; test setup invalid")
	}
	_ = EvaluateEnvelope([]dto.TrajectoryPoint{
		{XMM: -1000, YMM: 3000, ZMM: 500, TimeMS: 0, SpeedMMS: 100},
		{XMM: 1000, YMM: 3000, ZMM: 500, TimeMS: 1000, SpeedMMS: 100},
	}, 50, zones)
	if len(first) != firstLen {
		t.Fatalf("first evaluation result changed length after another evaluation: %d -> %d", firstLen, len(first))
	}
	for _, event := range first {
		if event.ZoneID != 1 {
			t.Fatalf("first evaluation result was polluted by a later evaluation: %+v", first)
		}
	}
}

func TestEvaluateEnvelopeDetectsHeightBandContact(t *testing.T) {
	points := []dto.TrajectoryPoint{
		{XMM: -1000, YMM: 0, ZMM: 70, TimeMS: 0, SpeedMMS: 100},
		{XMM: 1000, YMM: 0, ZMM: 70, TimeMS: 1000, SpeedMMS: 100},
	}
	zone := ZoneVolume{
		ID: 1, Name: "low ceiling", ZoneType: "restricted",
		Polygon:       Polygon{Ring: []Point2D{{X: -50, Y: -50}, {X: 50, Y: -50}, {X: 50, Y: 50}, {X: -50, Y: 50}, {X: -50, Y: -50}}},
		MinHeightMM:   0, MaxHeightMM: 50, SpeedLimitMMS: 0,
	}
	events := EvaluateEnvelope(points, 50, []ZoneVolume{zone})
	for _, event := range events {
		if event.ZoneID == 1 {
			return
		}
	}
	t.Fatalf("expected envelope contact when only the radius reaches the zone height band, got %+v", events)
}
