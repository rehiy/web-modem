package handler

import "testing"

func TestParseNetworkContextResponses(t *testing.T) {
	apnRaw := []string{
		`+CGDCONT: 1,"IP","cmnet","0.0.0.0",0,0,0,0`,
		`+CGDCONT: 2,"IPV4V6","iot.apn"`,
		"OK",
	}
	contexts := parseCGDCONT(apnRaw)
	if len(contexts) != 2 {
		t.Fatalf("expected 2 contexts, got %d", len(contexts))
	}
	if contexts[0].CID != 1 || contexts[0].PDPType != "IP" || contexts[0].APN != "cmnet" {
		t.Fatalf("unexpected first context: %+v", contexts[0])
	}
	if contexts[1].CID != 2 || contexts[1].PDPType != "IPV4V6" || contexts[1].APN != "iot.apn" {
		t.Fatalf("unexpected second context: %+v", contexts[1])
	}

	states := parseCGACT([]string{`+CGACT: 1,1`, `+CGACT: 2,0`, "OK"})
	if states[1] != 1 || states[2] != 0 {
		t.Fatalf("unexpected states: %+v", states)
	}

	ips := parseCGPADDR([]string{`+CGPADDR: 1,"10.10.1.2"`, `+CGPADDR: 2,"2409::1"`, "OK"}, 2)
	if len(ips) != 1 || ips[0].CID != 2 || ips[0].IP != "2409::1" {
		t.Fatalf("unexpected filtered IPs: %+v", ips)
	}
}

func TestParseSTKResponse(t *testing.T) {
	parsed := parseSTKResponse([]string{
		`+STIN: 6`,
		`+STGI: 1,"Balance"`,
		`+STGI: 2,"Package"`,
		"OK",
	})

	menu, ok := parsed["menu"].([]H)
	if !ok || len(menu) != 2 {
		t.Fatalf("expected menu items, got %#v", parsed["menu"])
	}
	if menu[0]["id"] != 1 || menu[0]["text"] != "Balance" {
		t.Fatalf("unexpected menu item: %+v", menu[0])
	}

	events, ok := parsed["events"].([]H)
	if !ok || len(events) != 1 || events[0]["code"] != "6" {
		t.Fatalf("unexpected events: %#v", parsed["events"])
	}
}

func TestSIMFileDecoders(t *testing.T) {
	if got := decodeICCID("986810123456789012F0"); got != "8986012143658709210" {
		t.Fatalf("unexpected ICCID decode: %s", got)
	}
	if got := decodeASCIIHex("546573742053504EFFFFFF"); got != "Test SPN" {
		t.Fatalf("unexpected ASCII decode: %s", got)
	}
	if got := cleanHex(`"AA BB:cc"`); got != "AABBcc" {
		t.Fatalf("unexpected clean hex: %s", got)
	}
}

func TestATResponseSupport(t *testing.T) {
	if !atCommandSupported([]string{"+CUSD: (0-2)", "OK"}, nil) {
		t.Fatal("expected command to be supported")
	}
	if atCommandSupported([]string{"ERROR"}, nil) {
		t.Fatal("expected final ERROR to be unsupported")
	}
}

func TestBuildCOPSCommand(t *testing.T) {
	act7 := 7

	tests := []struct {
		name     string
		mode     int
		format   int
		operator string
		act      *int
		wantCmd  string
		wantErr  bool
	}{
		// 前端默认请求：mode=0 自动，应忽略 format/operator
		{"auto", 0, 2, "", nil, "AT+COPS=0", false},
		{"deregister", 2, 2, "", nil, "AT+COPS=2", false},
		{"manual numeric", 1, 2, "46000", nil, `AT+COPS=1,2,"46000"`, false},
		{"manual numeric with act", 1, 2, "46000", &act7, `AT+COPS=1,2,"46000",7`, false},
		// 字母名称不应被 normalizeDigits 清空
		{"manual long name", 1, 0, "CHINA MOBILE", nil, `AT+COPS=1,0,"CHINA MOBILE"`, false},
		{"manual short name", 1, 1, "CMCC", &act7, `AT+COPS=1,1,"CMCC",7`, false},
		{"manual auto-preferred", 4, 2, "46000", nil, `AT+COPS=4,2,"46000"`, false},
		{"manual missing operator", 1, 2, "", nil, "", true},
		{"unsupported mode", 3, 2, "46000", nil, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildCOPSCommand(tt.mode, tt.format, tt.operator, tt.act)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got cmd %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantCmd {
				t.Fatalf("expected %q, got %q", tt.wantCmd, got)
			}
		})
	}
}

func TestParseCOPSOperators(t *testing.T) {
	raw := []string{
		`+COPS: (2,"CHINA MOBILE","CMCC","46000",7),(1,"CHN-UNICOM","UNICOM","46001",7),,(0-4),(0-2)`,
		"OK",
	}
	operators := parseCOPSOperators(raw)
	if len(operators) != 2 {
		t.Fatalf("expected 2 operators, got %+v", operators)
	}
	if operators[0].Status != 2 || operators[0].Numeric != "46000" || operators[0].Act != 7 {
		t.Fatalf("unexpected first operator: %+v", operators[0])
	}
	if operators[1].StatusLabel != "可用" || operators[1].ShortName != "UNICOM" {
		t.Fatalf("unexpected second operator: %+v", operators[1])
	}
}

func TestDecodeUSSDText(t *testing.T) {
	if got := decodeUSSDText("4F60597D", "72"); got != "你好" {
		t.Fatalf("unexpected UCS2 USSD decode: %s", got)
	}
	if got := decodeUSSDText("Balance: 10", "15"); got != "Balance: 10" {
		t.Fatalf("unexpected plain USSD decode: %s", got)
	}
	message, ok := firstUSSDMessage([]string{`+CUSD: 0,"4F60597D",72`, "OK"})
	if !ok || message.Decoded != "你好" || message.Status != "0" {
		t.Fatalf("unexpected parsed USSD: %+v", message)
	}
}

func TestPhonebookParsers(t *testing.T) {
	start, end, label := parseCPBRRange([]string{`+CPBR: (1-250),40,14`, "OK"})
	if start != 1 || end != 250 || label != "1-250" {
		t.Fatalf("unexpected range: %d %d %s", start, end, label)
	}
	entries := parseCPBREntries([]string{
		`+CPBR: 1,"13800138000",129,"Alice"`,
		`+CPBR: 2,"10086",129,"4E2D56FD79FB52A8"`,
		"OK",
	})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %+v", entries)
	}
	if entries[0].Name != "Alice" || entries[1].Name != "中国移动" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestParseCSIMResponse(t *testing.T) {
	parsed := parseCSIMResponse([]string{`+CSIM: 8,"AABB9000"`, "OK"})
	if parsed["length"] != 8 || parsed["data"] != "AABB" || parsed["sw1"] != "90" || parsed["sw2"] != "00" {
		t.Fatalf("unexpected CSIM parse: %+v", parsed)
	}
}
