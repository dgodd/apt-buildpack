package integration_test

import (
	"integration/cutlass"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Deploy", func() {
	var app *cutlass.App
	AfterEach(func() {
		if app != nil {
			app.Destroy()
		}
		app = nil
	})

	Context("simple", func() {
		BeforeEach(func() {
			app = cutlass.New(filepath.Join(bpDir, "fixtures", "simple"))
		})

		It("can use an apt dependency", func() {
			Expect(app.Push()).To(Succeed())
			Expect(app.InstanceStates()).To(Equal([]string{"RUNNING"}))
			Expect(app.Stdout.String()).To(ContainSubstring("Installing ascii"))

			Expect(app.GetBody("/")).To(ContainSubstring("ASCII 6/4 is decimal 100"))
		})
	})
})
