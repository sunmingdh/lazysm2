package tui

import "lazysm2/sm2"

type sm2Client interface {
	GetStatus() (sm2.Status, error)
	StartServiceOutput(string) (string, error)
	StartServiceOutputStream(name, port string, onLine func(string)) (string, error)
	StopServiceOutput(string) (string, error)
	StopAllOutput() (string, error)
	DebugService(string) (string, error)
	StartMode() string
	RefreshVPNStatus() bool
}

type realSM2Client struct{}

func (realSM2Client) GetStatus() (sm2.Status, error) {
	return sm2.GetStatus()
}

func (realSM2Client) StartServiceOutput(name string) (string, error) {
	return sm2.StartServiceOutput(name)
}

func (realSM2Client) StartServiceOutputStream(name, port string, onLine func(string)) (string, error) {
	return sm2.StartServiceOutputStream(name, port, onLine)
}

func (realSM2Client) StartMode() string {
	return sm2.StartMode()
}

func (realSM2Client) RefreshVPNStatus() bool {
	return sm2.RefreshVPNStatus()
}

func (realSM2Client) StopServiceOutput(name string) (string, error) {
	return sm2.StopServiceOutput(name)
}

func (realSM2Client) StopAllOutput() (string, error) {
	return sm2.StopAllOutput()
}

func (realSM2Client) DebugService(name string) (string, error) {
	return sm2.DebugService(name)
}

