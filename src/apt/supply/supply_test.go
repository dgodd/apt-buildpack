package supply_test

import (
	"apt/supply"
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/cloudfoundry/libbuildpack/ansicleaner"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -source=supply.go --destination=mocks_test.go --package=supply_test

var _ = Describe("Supply", func() {
	var (
		err         error
		buildDir    string
		depsDir     string
		depsIdx     string
		depDir      string
		supplier    *supply.Supplier
		logger      *libbuildpack.Logger
		buffer      *bytes.Buffer
		mockCtrl    *gomock.Controller
		mockCommand *MockCommand
	)

	BeforeEach(func() {
		depsDir, err = ioutil.TempDir("", "apt-buildpack.deps.")
		Expect(err).To(BeNil())

		buildDir, err = ioutil.TempDir("", "apt-buildpack.build.")
		Expect(err).To(BeNil())

		depsIdx = "14"
		depDir = filepath.Join(depsDir, depsIdx)

		err = os.MkdirAll(depDir, 0755)
		Expect(err).To(BeNil())

		buffer = new(bytes.Buffer)

		logger = libbuildpack.NewLogger(ansicleaner.New(buffer))

		mockCtrl = gomock.NewController(GinkgoT())
		mockCommand = NewMockCommand(mockCtrl)

		args := []string{buildDir, "", depsDir, depsIdx}
		stager := libbuildpack.NewStager(args, logger, &libbuildpack.Manifest{})

		supplier = supply.New(stager, mockCommand, logger)
	})

	AfterEach(func() {
		mockCtrl.Finish()

		err = os.RemoveAll(depsDir)
		Expect(err).To(BeNil())

		err = os.RemoveAll(buildDir)
		Expect(err).To(BeNil())
	})

	Describe("InstallApt", func() {
		BeforeEach(func() {
			ioutil.WriteFile(filepath.Join(buildDir, "Aptfile"), []byte(""), 0644)
		})
		BeforeEach(func() {
			mockCommand.EXPECT().Execute(
				"",
				gomock.Any(),
				ioutil.Discard,
				"apt-get",
				"-o", "debug::nolocking=true",
				"-o", "dir::cache="+supplier.AptCacheDir,
				"-o", "dir::state="+supplier.AptStateDir,
				"update",
			)
		})

		It("calls expected apt commands", func() {
			Expect(supplier.InstallApt()).To(Succeed())
		})
	})
})
