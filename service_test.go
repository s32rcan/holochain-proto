package holochain

import (
	"bytes"
	"fmt"
	ic "github.com/libp2p/go-libp2p-crypto"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	d := SetupTestDir()
	defer CleanupTestDir(d)

	Convey("we can detect an uninitialized directory", t, func() {
		So(IsInitialized(filepath.Join(d, DefaultDirectoryName)), ShouldBeFalse)
	})

	agent := "Fred Flintstone <fred@flintstone.com>"

	s, err := Init(filepath.Join(d, DefaultDirectoryName), AgentIdentity(agent))

	Convey("when initializing service in a directory", t, func() {
		So(err, ShouldBeNil)

		Convey("it should return a service with default values", func() {
			So(s.DefaultAgent.Identity(), ShouldEqual, AgentIdentity(agent))
			So(fmt.Sprintf("%v", s.Settings), ShouldEqual, "{true true bootstrap.holochain.net:10000 false}")
		})

		p := filepath.Join(d, DefaultDirectoryName)
		Convey("it should create agent files", func() {
			a, err := LoadAgent(p)
			So(err, ShouldBeNil)
			So(a.Identity(), ShouldEqual, AgentIdentity(agent))
		})

		Convey("we can detect that it was initialized", func() {
			So(IsInitialized(filepath.Join(d, DefaultDirectoryName)), ShouldBeTrue)
		})

		Convey("it should create an agent file", func() {
			a, err := readFile(p, AgentFileName)
			So(err, ShouldBeNil)
			So(string(a), ShouldEqual, agent)
		})
	})
}

func TestLoadService(t *testing.T) {
	d, service := setupTestService()
	root := service.Path
	defer CleanupTestDir(d)
	Convey("loading service from disk should set up the struct", t, func() {
		s, err := LoadService(root)
		So(err, ShouldBeNil)
		So(s.Path, ShouldEqual, root)
		So(s.Settings.DefaultPeerModeDHTNode, ShouldEqual, true)
		So(s.Settings.DefaultPeerModeAuthor, ShouldEqual, true)
		So(s.DefaultAgent.Identity(), ShouldEqual, AgentIdentity("Herbert <h@bert.com>"))
	})

}

func TestValidateServiceConfig(t *testing.T) {
	svc := ServiceConfig{}

	Convey("it should fail without one peer mode set to true", t, func() {
		err := svc.Validate()
		So(err.Error(), ShouldEqual, SysFileName+": At least one peer mode must be set to true.")
	})

	svc.DefaultPeerModeAuthor = true

	Convey("it should validate", t, func() {
		err := svc.Validate()
		So(err, ShouldBeNil)
	})

}

func TestConfiguredChains(t *testing.T) {
	d, s, h := setupTestChain("test")
	defer CleanupTestDir(d)

	Convey("Configured chains should return a hash of all the chains in the Service", t, func() {
		chains, err := s.ConfiguredChains()
		So(err, ShouldBeNil)
		So(chains["test"].nucleus.dna.UUID, ShouldEqual, h.nucleus.dna.UUID)
	})
}

func TestServiceGenChain(t *testing.T) {
	d, s, h := setupTestChain("test")
	defer CleanupTestDir(d)

	Convey("it should return a list of the chains", t, func() {
		list := s.ListChains()
		So(list, ShouldEqual, "installed holochains:     test <not-started>\n")
	})
	Convey("it should start a chain and return a holochain object", t, func() {
		h2, err := s.GenChain("test")
		So(err, ShouldBeNil)
		So(h2.nucleus.dna.UUID, ShouldEqual, h.nucleus.dna.UUID)
		list := s.ListChains()
		So(list, ShouldEqual, fmt.Sprintf("installed holochains:     test %v\n", h2.dnaHash))
	})
}

func TestCloneNew(t *testing.T) {
	d, s, h0 := setupTestChain("test")
	defer CleanupTestDir(d)

	name := "test2"
	root := filepath.Join(s.Path, name)

	orig := filepath.Join(s.Path, "test")

	agent, err := LoadAgent(s.Path)
	if err != nil {
		panic(err)
	}

	Convey("it should clone a chain by copying and creating an new UUID", t, func() {
		err = s.Clone(orig, root, agent, CloneWithNewUUID, InitializeDB)
		So(err, ShouldBeNil)

		So(dirExists(root, ChainDataDir), ShouldBeTrue)
		So(fileExists(root, ChainDataDir, StoreFileName), ShouldBeTrue)

		h, err := s.Load(name) // reload to confirm that it got saved correctly
		So(err, ShouldBeNil)

		So(h.nucleus.dna.Name, ShouldEqual, "test2")
		So(h.nucleus.dna.UUID, ShouldNotEqual, h0.nucleus.dna.UUID)

		agent, err := LoadAgent(s.Path)
		So(err, ShouldBeNil)
		So(h.agent.Identity(), ShouldEqual, agent.Identity())
		So(ic.KeyEqual(h.agent.PrivKey(), agent.PrivKey()), ShouldBeTrue)
		So(ic.KeyEqual(h.agent.PubKey(), agent.PubKey()), ShouldBeTrue)

		So(compareFile(filepath.Join(orig, "dna", "zySampleZome"), filepath.Join(h.DNAPath(), "zySampleZome"), "zySampleZome.zy"), ShouldBeTrue)

		So(h.rootPath, ShouldEqual, root)
		So(h.UIPath(), ShouldEqual, root+"/ui")
		So(h.DNAPath(), ShouldEqual, root+"/dna")
		So(h.DBPath(), ShouldEqual, root+"/db")

		So(compareFile(filepath.Join(orig, "ui"), h.UIPath(), "index.html"), ShouldBeTrue)
		So(compareFile(filepath.Join(orig, "dna", "zySampleZome"), filepath.Join(h.DNAPath(), "zySampleZome"), "profile.json"), ShouldBeTrue)
		So(compareFile(filepath.Join(orig, "dna"), h.DNAPath(), "properties_schema.json"), ShouldBeTrue)
		So(compareFile(orig, h.rootPath, ConfigFileName+".toml"), ShouldBeTrue)

		So(compareFile(filepath.Join(orig, ChainTestDir), filepath.Join(h.rootPath, ChainTestDir), "testSet1.json"), ShouldBeTrue)

		So(h.nucleus.dna.Progenitor.Identity, ShouldEqual, "Herbert <h@bert.com>")
		pk, _ := agent.PubKey().Bytes()
		So(string(h.nucleus.dna.Progenitor.PubKey), ShouldEqual, string(pk))
	})
}

func TestCloneJoin(t *testing.T) {
	d, s, h0 := setupTestChain("test")
	defer CleanupTestDir(d)

	name := "test2"
	root := filepath.Join(s.Path, name)

	orig := filepath.Join(s.Path, "test")

	agent, err := LoadAgent(s.Path)
	if err != nil {
		panic(err)
	}

	Convey("it should clone a chain by copying and without creating a new UUID", t, func() {
		err = s.Clone(orig, root, agent, CloneWithSameUUID, InitializeDB)
		So(err, ShouldBeNil)

		So(dirExists(root, ChainDataDir), ShouldBeTrue)
		So(fileExists(root, ChainDataDir, StoreFileName), ShouldBeTrue)

		h, err := s.Load(name) // reload to confirm that it got saved correctly
		So(err, ShouldBeNil)

		So(h.nucleus.dna.Name, ShouldEqual, "test")
		So(h.nucleus.dna.UUID, ShouldEqual, h0.nucleus.dna.UUID)
		agent, err := LoadAgent(s.Path)
		So(err, ShouldBeNil)
		So(h.agent.Identity(), ShouldEqual, agent.Identity())
		So(ic.KeyEqual(h.agent.PrivKey(), agent.PrivKey()), ShouldBeTrue)

		So(ic.KeyEqual(h.agent.PubKey(), agent.PubKey()), ShouldBeTrue)
		src, _ := readFile(orig, "dna", "zySampleZome.zy")
		dst, _ := readFile(root, "zySampleZome.zy")
		So(string(src), ShouldEqual, string(dst))
		So(fileExists(h.UIPath(), "index.html"), ShouldBeTrue)
		So(fileExists(h.DNAPath(), "zySampleZome", "profile.json"), ShouldBeTrue)
		So(fileExists(h.DNAPath(), "properties_schema.json"), ShouldBeTrue)
		So(fileExists(h.rootPath, ConfigFileName+".toml"), ShouldBeTrue)

		So(h.nucleus.dna.Progenitor.Identity, ShouldEqual, "Progenitor Agent <progenitore@example.com>")
		pk := []byte{8, 1, 18, 32, 193, 43, 31, 148, 23, 249, 163, 154, 128, 25, 237, 167, 253, 63, 214, 220, 206, 131, 217, 74, 168, 30, 215, 237, 231, 160, 69, 89, 48, 17, 104, 210}
		So(string(h.nucleus.dna.Progenitor.PubKey), ShouldEqual, string(pk))

	})
}

func TestCloneNoDB(t *testing.T) {
	d, s, _ := setupTestChain("test")
	defer CleanupTestDir(d)

	name := "test2"
	root := filepath.Join(s.Path, name)

	orig := filepath.Join(s.Path, "test")

	agent, err := LoadAgent(s.Path)
	if err != nil {
		panic(err)
	}

	Convey("it should create a chain from the examples directory", t, func() {
		err = s.Clone(orig, root, agent, CloneWithNewUUID, SkipInitializeDB)
		So(err, ShouldBeNil)

		So(dirExists(root, ChainDataDir), ShouldBeFalse)
		So(fileExists(root, ChainDNADir, "zySampleZome", "profile.json"), ShouldBeTrue)
	})
}

func TestGenDev(t *testing.T) {
	d, s := setupTestService()
	defer CleanupTestDir(d)
	name := "test"
	root := filepath.Join(s.Path, name)

	Convey("we detected unconfigured holochains", t, func() {
		f, err := s.IsConfigured(name)
		So(f, ShouldEqual, "")
		So(err.Error(), ShouldEqual, "No DNA file in "+filepath.Join(root, ChainDNADir)+"/")
		_, err = s.load("test", "json")
		So(err.Error(), ShouldEqual, "open "+filepath.Join(root, ChainDNADir, DNAFileName+".json")+": no such file or directory")

	})

	Convey("when generating a dev holochain", t, func() {
		h, err := s.GenDev(root, "json", InitializeDB)
		So(err, ShouldBeNil)

		f, err := s.IsConfigured(name)
		So(err, ShouldBeNil)
		So(f, ShouldEqual, "json")

		h, err = s.Load(name)
		So(err, ShouldBeNil)

		lh, err := s.load(name, "json")
		So(err, ShouldBeNil)
		So(lh.nodeID, ShouldEqual, h.nodeID)
		So(lh.nodeIDStr, ShouldEqual, h.nodeIDStr)
		So(lh.config.Port, ShouldEqual, DefaultPort)
		So(h.config.PeerModeDHTNode, ShouldEqual, s.Settings.DefaultPeerModeDHTNode)
		So(h.config.PeerModeAuthor, ShouldEqual, s.Settings.DefaultPeerModeAuthor)
		So(h.config.BootstrapServer, ShouldEqual, s.Settings.DefaultBootstrapServer)
		So(h.config.EnableMDNS, ShouldEqual, s.Settings.DefaultEnableMDNS)

		So(dirExists(root), ShouldBeTrue)
		So(dirExists(h.DNAPath()), ShouldBeTrue)
		So(dirExists(h.TestPath()), ShouldBeTrue)
		So(dirExists(h.UIPath()), ShouldBeTrue)
		So(fileExists(h.TestPath(), "sampleScenario", "listener.json"), ShouldBeTrue)
		So(fileExists(h.DNAPath(), "zySampleZome", "profile.json"), ShouldBeTrue)
		So(fileExists(h.UIPath(), "index.html"), ShouldBeTrue)
		So(fileExists(h.UIPath(), "hc.js"), ShouldBeTrue)
		So(fileExists(h.rootPath, ConfigFileName+".json"), ShouldBeTrue)

		Convey("we should not be able re generate it", func() {
			_, err = s.GenDev(root, "json", SkipInitializeDB)
			So(err.Error(), ShouldEqual, "holochain: "+root+" already exists")
		})
	})
}

func TestSaveScaffold(t *testing.T) {
	d, s := setupTestService()
	defer CleanupTestDir(d)
	name := "test"
	root := filepath.Join(s.Path, name)

	Convey("it should write out a scaffold file to a directory tree with JSON encoding", t, func() {
		scaffoldBlob := bytes.NewBuffer([]byte(BasicTemplateScaffold))

		scaffold, err := s.SaveScaffold(scaffoldBlob, root, "appName", "json", false)
		So(err, ShouldBeNil)
		So(scaffold, ShouldNotBeNil)
		So(scaffold.ScaffoldVersion, ShouldEqual, ScaffoldVersion)
		So(scaffold.DNA.Name, ShouldEqual, "appName")
		So(dirExists(root), ShouldBeTrue)
		So(dirExists(root, ChainDNADir), ShouldBeTrue)
		So(dirExists(root, ChainUIDir), ShouldBeTrue)
		So(dirExists(root, ChainTestDir), ShouldBeTrue)
		So(dirExists(root, ChainTestDir, scaffold.Scenarios[0].Name), ShouldBeTrue)
		So(fileExists(root, ChainTestDir, scaffold.Scenarios[0].Name, scaffold.Scenarios[0].Roles[0].Name+".json"), ShouldBeTrue)
		So(fileExists(root, ChainTestDir, scaffold.Scenarios[0].Name, scaffold.Scenarios[0].Roles[1].Name+".json"), ShouldBeTrue)
		So(fileExists(root, ChainTestDir, scaffold.Scenarios[0].Name, "_config.json"), ShouldBeTrue)

		So(dirExists(root, ChainDNADir, "sampleZome"), ShouldBeTrue)
		So(fileExists(root, ChainDNADir, "sampleZome", "sampleEntry.json"), ShouldBeTrue)
		So(fileExists(root, ChainDNADir, "sampleZome", "sampleZome.js"), ShouldBeTrue)
		So(fileExists(root, ChainDNADir, DNAFileName+".json"), ShouldBeTrue)
		So(fileExists(root, ChainDNADir, "properties_schema.json"), ShouldBeTrue)
		So(fileExists(root, ChainTestDir, "sample.json"), ShouldBeTrue)
		So(fileExists(root, ChainUIDir, "index.html"), ShouldBeTrue)
		So(fileExists(root, ChainUIDir, "hc.js"), ShouldBeTrue)
	})

	Convey("it should write out a scaffold file to a directory tree with toml encoding", t, func() {
		scaffoldBlob := bytes.NewBuffer([]byte(BasicTemplateScaffold))

		root2 := filepath.Join(s.Path, name+"2")

		scaffold, err := s.SaveScaffold(scaffoldBlob, root2, "appName", "toml", false)
		So(err, ShouldBeNil)
		So(scaffold, ShouldNotBeNil)
		So(scaffold.ScaffoldVersion, ShouldEqual, ScaffoldVersion)
		So(dirExists(root2), ShouldBeTrue)
		So(fileExists(root2, ChainDNADir, DNAFileName+".toml"), ShouldBeTrue)
		// the reset of the files are still saved as json...
	})
}

func TestMakeConfig(t *testing.T) {
	d, s := setupTestService()
	defer CleanupTestDir(d)
	h := &Holochain{encodingFormat: "json", rootPath: d}
	Convey("make config should produce default values", t, func() {
		err := makeConfig(h, s)
		So(err, ShouldBeNil)
		So(h.config.Port, ShouldEqual, DefaultPort)
		So(h.config.EnableMDNS, ShouldBeFalse)
		So(h.config.BootstrapServer, ShouldNotEqual, "")
		So(h.config.Loggers.App.Format, ShouldEqual, "%{color:cyan}%{message}")

	})
	Convey("make config should produce default config from OS env overridden values", t, func() {
		os.Setenv("HOLOCHAINCONFIG_PORT", "12345")
		os.Setenv("HOLOCHAINCONFIG_ENABLEMDNS", "true")
		os.Setenv("HOLOCHAINCONFIG_LOGPREFIX", "prefix:")
		os.Setenv("HOLOCHAINCONFIG_BOOTSTRAP", "_")
		err := makeConfig(h, s)
		So(err, ShouldBeNil)
		So(h.config.Port, ShouldEqual, 12345)
		So(h.config.EnableMDNS, ShouldBeTrue)
		So(h.config.Loggers.App.Format, ShouldEqual, "%{color:cyan}%{message}")
		So(h.config.Loggers.App.Prefix, ShouldEqual, "prefix:")
		So(h.config.BootstrapServer, ShouldEqual, "")
	})
}
