package webrtc

import (
	"testing"
)

func TestABRController_Evaluation(t *testing.T) {
	abr := NewABRController()

	// High quality conditions: 1.2 Mbps, 0.5% loss -> High ('f')
	if layer := abr.EvaluateLayer(1200000, 0.5); layer != LayerHigh {
		t.Fatalf("Expected LayerHigh ('f'), got '%s'", layer)
	}

	// Medium quality conditions: 500 kbps, 1.0% loss -> Medium ('h')
	if layer := abr.EvaluateLayer(500000, 1.0); layer != LayerMedium {
		t.Fatalf("Expected LayerMedium ('h'), got '%s'", layer)
	}

	// Poor bandwidth: 200 kbps, 0.0% loss -> Low ('q')
	if layer := abr.EvaluateLayer(200000, 0.0); layer != LayerLow {
		t.Fatalf("Expected LayerLow ('q'), got '%s'", layer)
	}

	// High packet loss: 1.5 Mbps, 8.0% loss -> Low ('q')
	if layer := abr.EvaluateLayer(1500000, 8.0); layer != LayerLow {
		t.Fatalf("Expected LayerLow ('q'), got '%s'", layer)
	}
}

func TestDynacastEngine_SubscriberTracking(t *testing.T) {
	dyn := NewDynacastEngine()
	roomID := "room-dynacast-1"

	// 1. First subscriber joins 'f' -> should signal resume
	shouldResume := dyn.AddSubscriber(roomID, LayerHigh)
	if !shouldResume {
		t.Fatalf("Expected shouldResume=true for first subscriber on 'f'")
	}

	// 2. Second subscriber joins 'f' -> should not signal resume again
	shouldResume = dyn.AddSubscriber(roomID, LayerHigh)
	if shouldResume {
		t.Fatalf("Expected shouldResume=false for second subscriber on 'f'")
	}

	// 3. One subscriber leaves 'f' -> count is 1, should not signal pause
	shouldPause := dyn.RemoveSubscriber(roomID, LayerHigh)
	if shouldPause {
		t.Fatalf("Expected shouldPause=false when 1 subscriber remains on 'f'")
	}

	// 4. Last subscriber switches from 'f' to 'h' -> should pause 'f' and resume 'h'
	resumeH, pauseF := dyn.SwitchSubscriber(roomID, LayerHigh, LayerMedium)
	if !pauseF {
		t.Fatalf("Expected pauseF=true when 'f' reaches 0 subscribers")
	}
	if !resumeH {
		t.Fatalf("Expected resumeH=true when 'h' gets its first subscriber")
	}

	counts := dyn.GetLayerCounts(roomID)
	if counts[LayerHigh] != 0 || counts[LayerMedium] != 1 {
		t.Fatalf("Unexpected layer counts: %+v", counts)
	}
}
