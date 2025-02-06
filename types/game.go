package types

type Match struct {
	MapName     string
	TickRate    float32
	Events      []Event
	Date        string
	PlayerStats map[uint64]*PlayerStats
}

type Event struct {
	Type string
	Data map[string]interface{}
}
