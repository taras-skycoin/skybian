package prepconf

import (
	"encoding/json"
	"os"

	"github.com/SkycoinProject/dmsg/cipher"
	"github.com/SkycoinProject/skywire-mainnet/pkg/app/appcommon"
	"github.com/SkycoinProject/skywire-mainnet/pkg/hypervisor"
	"github.com/SkycoinProject/skywire-mainnet/pkg/restart"
	"github.com/SkycoinProject/skywire-mainnet/pkg/routing"
	"github.com/SkycoinProject/skywire-mainnet/pkg/skyenv"
	"github.com/SkycoinProject/skywire-mainnet/pkg/visor"

	"github.com/SkycoinProject/skybian/pkg/boot"
)

// Config configures how hypervisor and visor images are to be generated.
type Config struct {
	VisorConf      string
	HypervisorConf string
	TLSCert        string
	TLSKey         string
}

// Prepare prepares either a hypervisor and visor config file (based on provided
// conf and boot parameters).
func Prepare(conf Config, bp boot.Params) error {

	// generate config struct
	type genFn func(conf Config, bp boot.Params) (out interface{}, err error)

	// ensure config file of 'name' exists
	// if not, write config generated by 'genConfig'
	ensureExists := func(name string, genConfig genFn) error {
		//// Do nothing if file exists.
		if _, err := os.Stat(name); err == nil {
			return nil
		}
		// Create file.
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE, 0644) //nolint:gosec
		if err != nil {
			return err
		}
		// Generate and write config to file.
		conf, err := genConfig(conf, bp)
		if err != nil {
			return err
		}
		raw, err := json.MarshalIndent(conf, "", "\t")
		if err != nil {
			return err
		}
		_, err = f.Write(raw)
		if err1 := f.Close(); err == nil {
			err = err1
		}
		return err
	}

	// config location and contents depend on mode
	switch bp.Mode {
	case boot.HypervisorMode:
		return ensureExists(conf.HypervisorConf, generateHypervisorConfig)
	case boot.VisorMode:
		return ensureExists(conf.VisorConf, generateVisorConfig)
	default:
		return boot.ErrInvalidMode
	}
}

func genKeyPair(bp boot.Params) (pk cipher.PubKey, sk cipher.SecKey, err error) {
	if sk = bp.LocalSK; sk.Null() {
		pk, sk = cipher.GenerateKeyPair()
	} else {
		pk, err = sk.PubKey()
	}
	return
}

func generateVisorConfig(_ Config, bp boot.Params) (interface{}, error) {
	skysocksArgs := func() (args []string) {
		if bp.SkysocksPasscode != "" {
			args = []string{"-passcode", bp.SkysocksPasscode}
		}
		return args
	}
	hypervisors := func() (hvs []visor.HypervisorConfig) {
		for _, pk := range bp.HypervisorPKs {
			hvs = append(hvs, visor.HypervisorConfig{PubKey: pk})
		}
		return hvs
	}
	pk, sk, err := genKeyPair(bp)
	if err != nil {
		return nil, err
	}
	out := new(visor.Config)

	out.Version = "1.0"
	out.KeyPair = &visor.KeyPair{
		PubKey: pk,
		SecKey: sk,
	}
	if out.STCP, err = visor.DefaultSTCPConfig(); err != nil {
		return nil, err
	}
	out.Dmsg = visor.DefaultDmsgConfig()
	out.DmsgPty = visor.DefaultDmsgPtyConfig()
	out.DmsgPty.AuthFile = "/var/skywire-visor/dsmgpty/whitelist.json"
	out.DmsgPty.CLIAddr = "/run/skywire-visor/dmsgpty/cli.sock"
	out.Transport = visor.DefaultTransportConfig()
	out.Transport.LogStore.Location = "/var/skywire-visor/transports"
	out.Routing = visor.DefaultRoutingConfig()
	out.UptimeTracker = visor.DefaultUptimeTrackerConfig()
	out.Hypervisors = hypervisors()
	out.LogLevel = visor.DefaultLogLevel
	out.ShutdownTimeout = visor.DefaultTimeout
	out.RestartCheckDelay = restart.DefaultCheckDelay.String()
	out.Interfaces = visor.DefaultInterfaceConfig()
	out.AppServerAddr = appcommon.DefaultServerAddr
	out.AppsPath = "/usr/bin/apps"
	out.LocalPath = "/var/skywire-visor/apps"
	out.Apps = []visor.AppConfig{
		{
			App:       skyenv.SkychatName,
			AutoStart: true,
			Port:      routing.Port(skyenv.SkychatPort),
			Args:      []string{"-addr", skyenv.SkychatAddr},
		},
		{
			App:       skyenv.SkysocksName,
			AutoStart: true,
			Port:      routing.Port(skyenv.SkysocksPort),
			Args:      skysocksArgs(),
		},
		{
			App:       skyenv.SkysocksClientName,
			AutoStart: false,
			Port:      routing.Port(skyenv.SkysocksClientPort),
			Args:      []string{"-addr", skyenv.SkysocksClientAddr},
		},
	}
	return out, nil
}

func generateHypervisorConfig(conf Config, bp boot.Params) (interface{}, error) {
	pk, sk, err := genKeyPair(bp)
	if err != nil {
		return nil, err
	}
	out := new(hypervisor.Config)
	out.PK = pk
	out.SK = sk
	out.DBPath = "/var/skywire-hypervisor/users.db"
	out.EnableAuth = true
	out.Cookies.BlockKey = cipher.RandByte(32)
	out.Cookies.HashKey = cipher.RandByte(64)
	out.Cookies.FillDefaults()
	out.DmsgDiscovery = skyenv.DefaultDmsgDiscAddr
	out.DmsgPort = skyenv.DmsgHypervisorPort
	out.HTTPAddr = ":8000"
	out.EnableTLS = true
	// TODO(evanlinjin): Pass filenames as cli args in 'skyconf'.
	out.TLSCertFile = conf.TLSCert
	out.TLSKeyFile = conf.TLSKey
	err = GenCert(out.TLSCertFile, out.TLSKeyFile)
	return out, err
}