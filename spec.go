package main

type SlotConfig struct {
	temp    float64
	speed   int
	dynamic bool
}

var config = []SlotConfig{
	SlotConfig{0, 0, false},
	SlotConfig{51, 15, false},
	SlotConfig{56, 20, false},
	SlotConfig{65, 25, false},
	SlotConfig{70, 35, true},
	SlotConfig{75, 40, true},
	SlotConfig{80, 50, true},
	SlotConfig{85, 70, true},
	SlotConfig{90, 90, true},
	SlotConfig{95, 100, true},
}

type SlotEntry struct {
	from   float64
	to     float64
	speed  int
	speed2 int
}

type Spec []SlotEntry

func NewSpec(config []SlotConfig) Spec {
	var res []SlotEntry
	for i, c := range config {
		if i < len(config)-1 {
			next := config[i+1]
			speed2 := c.speed
			if c.dynamic {
				speed2 = next.speed
			}
			res = append(res, SlotEntry{
				from:   c.temp,
				to:     next.temp,
				speed:  c.speed,
				speed2: speed2,
			})
		} else {
			res = append(res, SlotEntry{
				from:   c.temp,
				to:     1000,
				speed:  c.speed,
				speed2: 100,
			})
		}
	}
	return res
}

func (s *Spec) find(temp float64) *SlotEntry {
	var entry *SlotEntry = nil
	for _, e := range *s {
		if temp > e.from {
			entry = &e
		} else {
			break
		}
	}
	return entry
}

func (e *SlotEntry) isInSlot(temp, threshold float64) bool {
	return temp > e.from-threshold && temp < e.to+threshold
}

func (e *SlotEntry) getAdjustedSpeed(temp float64) int {
	fan := e.speed
	if e.speed2 > fan {
		// adjust speed if dynamic
		adjust := (temp - e.from) / (e.to - e.from)
		// fmt.Fprintf(os.Stderr, "  adjust: %d + %d * %.2f\n", e.speed, e.speed2-e.speed, adjust)
		fan += int(float64(e.speed2-fan) * adjust)
	}
	return fan
}
