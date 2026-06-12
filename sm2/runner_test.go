package sm2

import (
	"reflect"
	"testing"
)

func TestParseStatusOutput(t *testing.T) {
	output := `+-------------------+-----------+---------+-------+--------+
| Name              | Version   | PID     | Port  | Status |
+-------------------+-----------+---------+-------+--------+
| SERVICE-DB        |           | 0       | 9001  |  PASS  |
| SERVICE-ALPHA     | 0.1.0     | 10001   | 8001  |  PASS  |
| SERVICE-BETA      | 1.0.0     | 10002   | 8002  |  PASS  |
| SERVICE-GAMMA     | 2.0.0     | 10003   | 8003  |  PASS  |
| SERVICE-DELTA     | 3.0.0     | 10004   | 8004  |  PASS  |
| SERVICE-EPSILON   | 4.0.0     | 10005   | 8005  |  PASS  |
+-------------------+-----------+---------+-------+--------+

Also, the following processes are running which occupy ports of services
that are defined in service manager config:

These might include entirely separate processes running on your machine,
or they could be services running from inside your IDE or by other means.
Please note: You will not be able to manage these services using sm2.
+---------+-------+--------------------+
| PID     | Port  | Reserved by        |
+---------+-------+--------------------+
| 10006   | 8006  | EXTERNAL-SERVICE-X |
+---------+-------+--------------------+`

	expected := Status{
		Services: []Service{
			{Name: "SERVICE-DB", Version: "", PID: "0", Port: "9001", Status: "PASS", IsRunning: true},
			{Name: "SERVICE-ALPHA", Version: "0.1.0", PID: "10001", Port: "8001", Status: "PASS", IsRunning: true},
			{Name: "SERVICE-BETA", Version: "1.0.0", PID: "10002", Port: "8002", Status: "PASS", IsRunning: true},
			{Name: "SERVICE-GAMMA", Version: "2.0.0", PID: "10003", Port: "8003", Status: "PASS", IsRunning: true},
			{Name: "SERVICE-DELTA", Version: "3.0.0", PID: "10004", Port: "8004", Status: "PASS", IsRunning: true},
			{Name: "SERVICE-EPSILON", Version: "4.0.0", PID: "10005", Port: "8005", Status: "PASS", IsRunning: true},
		},
		OccupiedPorts: []OccupiedPort{
			{PID: "10006", Port: "8006", ServiceName: "EXTERNAL-SERVICE-X"},
		},
	}

	result, err := parseStatusOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("expected %+v, got %+v", expected, result)
	}
}

func TestParseStatusOutputJoinsWrappedServiceNames(t *testing.T) {
	// sm2 --status wraps long service names onto a continuation row when the
	// terminal is too narrow. The continuation row has only the name fragment;
	// all other columns (version, PID, port, status) are empty.
	output := `+---------------------------------------+-----------+---------+-------+--------+
| Name                                  | Version   | PID     | Port  | Status |
+---------------------------------------+-----------+---------+-------+--------+
| SERVICE-A                             | 1.0.0     | 10001   | 8001  |  PASS  |
| LONG_SERVICE_ALPHA_REGISTRATION_FRONT | 0.1.0     | 10002   | 8002  |  PASS  |
| END                                   |           |         |       |        |
| LONG_SERVICE_BETA_IDENTIFICATION_FRONT| 0.2.0     | 10003   | 8003  |  PASS  |
| END                                   |           |         |       |        |
+---------------------------------------+-----------+---------+-------+--------+`

	result, err := parseStatusOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []Service{
		{Name: "SERVICE-A", Version: "1.0.0", PID: "10001", Port: "8001", Status: "PASS", IsRunning: true},
		{Name: "LONG_SERVICE_ALPHA_REGISTRATION_FRONTEND", Version: "0.1.0", PID: "10002", Port: "8002", Status: "PASS", IsRunning: true},
		{Name: "LONG_SERVICE_BETA_IDENTIFICATION_FRONTEND", Version: "0.2.0", PID: "10003", Port: "8003", Status: "PASS", IsRunning: true},
	}
	if !reflect.DeepEqual(result.Services, expected) {
		t.Errorf("services = %+v, want %+v", result.Services, expected)
	}
}

func TestStdoutLogPathFromDebugOutput(t *testing.T) {
	output := `Log files in /Users/me/.sm2/install/example-service/example-service-1.0.0/logs:
	example-service.log  7410494
	          stdout.log  8133739`

	got, err := StdoutLogPathFromDebugOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "/Users/me/.sm2/install/example-service/example-service-1.0.0/logs/stdout.log"
	if got != want {
		t.Fatalf("stdout log path = %q, want %q", got, want)
	}
}

func TestParseProfilesOutputIncludesProfileServices(t *testing.T) {
	output := `Searching for (.*..*)...
[PROFILE] PROFILE-ALPHA                                                  -> (3 services)
  - SERVICE-A
  - SERVICE-B
  - SERVICE-C
[SERVICE] SERVICE-A                                                      -> service-a ::: git@github.com:example/service-a.git
[PROFILE] PROFILE-BETA                                                   -> (2 services)
  - SERVICE-D
  - SERVICE-E`

	profiles, profileServices := parseProfilesOutput(output)

	wantProfiles := []string{"PROFILE-ALPHA", "PROFILE-BETA"}
	if !reflect.DeepEqual(profiles, wantProfiles) {
		t.Fatalf("profiles = %+v, want %+v", profiles, wantProfiles)
	}

	wantServices := map[string][]string{
		"PROFILE-ALPHA": {"SERVICE-A", "SERVICE-B", "SERVICE-C"},
		"PROFILE-BETA":  {"SERVICE-D", "SERVICE-E"},
	}
	if !reflect.DeepEqual(profileServices, wantServices) {
		t.Fatalf("profile services = %+v, want %+v", profileServices, wantServices)
	}
}
