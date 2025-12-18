package modem

type ModemInfo struct {
	Port         string `json:"port"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	IMEI         string `json:"imei"`
	PhoneNumber  string `json:"phoneNumber"`
	IMSI         string `json:"imsi"`
	Operator     string `json:"operator"`
	Connected    bool   `json:"connected"`
}

type ATCommand struct {
	Port     string `json:"port,omitempty"`
	Command  string `json:"command"`
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

type SignalStrength struct {
	RSSI    int    `json:"rssi"`
	Quality int    `json:"quality"`
	DBM     string `json:"dbm"`
}

type SMS struct {
	Index   int    `json:"index"`
	Status  string `json:"status"`
	Number  string `json:"number"`
	Time    string `json:"time"`
	Message string `json:"message"`
}

type SendSMSRequest struct {
	Port    string `json:"port,omitempty"`
	Number  string `json:"number"`
	Message string `json:"message"`
}

type SerialPort struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Connected bool   `json:"connected"`
}
