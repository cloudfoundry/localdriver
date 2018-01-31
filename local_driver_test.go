package localdriver_test

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/goshims/filepathshim"
	"code.cloudfoundry.org/goshims/filepathshim/filepath_fake"
	"code.cloudfoundry.org/goshims/osshim"
	"code.cloudfoundry.org/goshims/osshim/os_fake"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/localdriver"
	"code.cloudfoundry.org/localdriver/oshelper"
	"code.cloudfoundry.org/voldriver"
	"code.cloudfoundry.org/voldriver/driverhttp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Local Driver", func() {
	var (
		testLogger     lager.Logger
		ctx            context.Context
		env            voldriver.Env
		localDriver    *localdriver.LocalDriver
		mountDir       string
		volumeId       string
		err            error
		expectedVolume string
		expectedMounts string
	)
	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("localdriver-local")
		ctx = context.TODO()
		env = driverhttp.NewHttpDriverEnv(testLogger, ctx)

		testOs := &osshim.OsShim{}
		testFilepath := &filepathshim.FilepathShim{}

		mountDir, err = ioutil.TempDir("", "localDrivertest")
		Expect(err).ToNot(HaveOccurred())

		localDriver = localdriver.NewLocalDriver(testOs, testFilepath, mountDir, oshelper.NewOsHelper())
		volumeId = "test-volume-id"

		expectedVolume = filepath.Join(mountDir, "_volumes", "test-volume-id")
		expectedMounts = filepath.Join(mountDir, "_mounts", "test-volume-id")
	})

	AfterEach(func() {
		os.RemoveAll(mountDir)
	})

	Describe("#Activate", func() {
		It("returns Implements: VolumeDriver", func() {
			activateResponse := localDriver.Activate(env)
			Expect(len(activateResponse.Implements)).To(BeNumerically(">", 0))
			Expect(activateResponse.Implements[0]).To(Equal("VolumeDriver"))
		})
	})

	Describe("Mount", func() {

		Context("when the volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(env, localDriver, volumeId)
				mountSuccessful(env, localDriver, volumeId)
			})

			AfterEach(func() {
				localDriver.Remove(env, voldriver.RemoveRequest{
					Name: volumeId,
				})
			})

			Context("when the volume exists", func() {
				AfterEach(func() {
					unmountSuccessful(env, localDriver, volumeId)
				})

				It("should mount the volume on the local filesystem", func() {
					fromExists, err := exists(expectedVolume)
					Expect(err).NotTo(HaveOccurred())
					Expect(fromExists).To(BeTrue())
					toExists, err := exists(expectedMounts)
					Expect(err).NotTo(HaveOccurred())
					Expect(toExists).To(BeTrue())
				})

				It("returns the mount point on a /VolumeDriver.Get response", func() {
					getResponse := getSuccessful(env, localDriver, volumeId)
					Expect(getResponse.Volume.Mountpoint).To(Equal(expectedMounts))
				})
			})

			Context("when the volume is missing", func() {
				BeforeEach(func() {
					mountSuccessful(env, localDriver, volumeId)
					expectedVolume = filepath.Join(mountDir, "_volumes", "test-volume-id")
				})

				It("returns an error", func() {
					os.RemoveAll(expectedVolume)

					mountResponse := localDriver.Mount(env, voldriver.MountRequest{
						Name: volumeId,
					})
					Expect(mountResponse.Err).To(Equal("Volume 'test-volume-id' is missing"))
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				mountResponse := localDriver.Mount(env, voldriver.MountRequest{
					Name: "bla",
				})
				Expect(mountResponse.Err).To(Equal("Volume 'bla' must be created before being mounted"))
			})
		})
	})

	Describe("Unmount", func() {
		Context("when a volume has been created", func() {

			BeforeEach(func() {
				createSuccessful(env, localDriver, volumeId)
			})

			AfterEach(func() {
				localDriver.Remove(env, voldriver.RemoveRequest{
					Name: volumeId,
				})
			})

			Context("when a volume has been mounted", func() {
				BeforeEach(func() {
					mountSuccessful(env, localDriver, volumeId)
				})

				It("After unmounting /VolumeDriver.Get returns no mountpoint", func() {
					unmountSuccessful(env, localDriver, volumeId)
					getResponse := getSuccessful(env, localDriver, volumeId)
					Expect(getResponse.Volume.Mountpoint).To(Equal(""))
				})

				It("/VolumeDriver.Unmount doesn't remove mountpath from OS", func() {
					unmountSuccessful(env, localDriver, volumeId)
					removedExists, err := exists(expectedMounts)
					Expect(err).NotTo(HaveOccurred())
					Expect(removedExists).NotTo(BeTrue())
				})

				Context("when the same volume is mounted a second time then unmounted", func() {
					BeforeEach(func() {
						mountSuccessful(env, localDriver, volumeId)
						unmountSuccessful(env, localDriver, volumeId)
					})

					It("should not report empty mountpoint until unmount is called again", func() {
						getResponse := getSuccessful(env, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))

						unmountSuccessful(env, localDriver, volumeId)
						getResponse = getSuccessful(env, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).To(Equal(""))
					})
				})

				Context("when the mountpath is not found on the filesystem", func() {
					var unmountResponse voldriver.ErrorResponse

					BeforeEach(func() {
						os.RemoveAll(expectedMounts)
						unmountResponse = localDriver.Unmount(env, voldriver.UnmountRequest{
							Name: volumeId,
						})
					})

					It("returns an error", func() {
						Expect(unmountResponse.Err).To(ContainSubstring("Volume " + volumeId + " does not exist"))
					})

					It("/VolumeDriver.Get still returns the mountpoint", func() {
						getResponse := getSuccessful(env, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))
					})
				})

				Context("when the mountpath cannot be accessed", func() {
					var (
						unmountResponse voldriver.ErrorResponse
						fakeOs          *os_fake.FakeOs
						fakeFilepath    *filepath_fake.FakeFilepath
					)

					BeforeEach(func() {
						fakeOs = &os_fake.FakeOs{}
						fakeFilepath = &filepath_fake.FakeFilepath{}

						state := make(map[string]*localdriver.LocalVolumeInfo)
						volInfo := &localdriver.LocalVolumeInfo{VolumeInfo: voldriver.VolumeInfo{Name: volumeId, Mountpoint: "/path/to/mount/_mounts/some-volume-id"}}
						state[volumeId] = volInfo

						localDriver = localdriver.NewLocalDriverWithState(state, fakeOs, fakeFilepath, mountDir, oshelper.NewOsHelper())
						fakeOs.StatReturns(nil, errors.New("something bad"))
						unmountResponse = localDriver.Unmount(env, voldriver.UnmountRequest{
							Name: volumeId,
						})
					})

					It("returns an error", func() {
						Expect(unmountResponse.Err).To(Equal("Error establishing whether volume exists"))
					})

					It("/VolumeDriver.Get still returns the mountpoint", func() {
						getResponse := getSuccessful(env, localDriver, volumeId)
						Expect(getResponse.Volume.Mountpoint).NotTo(Equal(""))
					})
				})
			})

			Context("when the volume has not been mounted", func() {
				It("returns an error", func() {
					unmountResponse := localDriver.Unmount(env, voldriver.UnmountRequest{
						Name: volumeId,
					})

					Expect(unmountResponse.Err).To(Equal("Volume not previously mounted"))
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				unmountResponse := localDriver.Unmount(env, voldriver.UnmountRequest{
					Name: volumeId,
				})

				Expect(unmountResponse.Err).To(Equal(fmt.Sprintf("Volume '%s' not found", volumeId)))
			})
		})
	})

	Describe("Create", func() {
		Context("when a second create is called with the same volume ID", func() {
			BeforeEach(func() {
				createSuccessful(env, localDriver, "volume")
			})

			Context("with the same opts", func() {
				It("does nothing", func() {
					createSuccessful(env, localDriver, "volume")
				})
			})
		})
	})

	Describe("Get", func() {
		Context("when the volume has been created", func() {
			It("returns the volume name", func() {
				createSuccessful(env, localDriver, volumeId)
				getSuccessful(env, localDriver, volumeId)
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				getUnsuccessful(env, localDriver, volumeId)
			})
		})
	})

	Describe("Path", func() {
		Context("when a volume is mounted", func() {
			BeforeEach(func() {
				createSuccessful(env, localDriver, volumeId)
				mountSuccessful(env, localDriver, volumeId)
			})

			It("returns the mount point on a /VolumeDriver.Path", func() {
				pathResponse := localDriver.Path(env, voldriver.PathRequest{
					Name: volumeId,
				})
				Expect(pathResponse.Err).To(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(expectedMounts))
			})
		})

		Context("when a volume is not created", func() {
			It("returns an error on /VolumeDriver.Path", func() {
				pathResponse := localDriver.Path(env, voldriver.PathRequest{
					Name: "volume-that-does-not-exist",
				})
				Expect(pathResponse.Err).NotTo(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(""))
			})
		})

		Context("when a volume is created but not mounted", func() {
			var (
				volumeName string
			)
			BeforeEach(func() {
				volumeName = "my-volume"
				createSuccessful(env, localDriver, volumeName)
			})

			It("returns an error on /VolumeDriver.Path", func() {
				pathResponse := localDriver.Path(env, voldriver.PathRequest{
					Name: "volume-that-does-not-exist",
				})
				Expect(pathResponse.Err).NotTo(Equal(""))
				Expect(pathResponse.Mountpoint).To(Equal(""))
			})
		})
	})

	Describe("List", func() {
		Context("when there are volumes", func() {
			BeforeEach(func() {
				createSuccessful(env, localDriver, volumeId)
			})

			It("returns the list of volumes", func() {
				listResponse := localDriver.List(env)

				Expect(listResponse.Err).To(Equal(""))
				Expect(listResponse.Volumes[0].Name).To(Equal(volumeId))

			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				volumeName := "test-volume-2"
				getUnsuccessful(env, localDriver, volumeName)
			})
		})
	})

	Describe("Remove", func() {
		It("should fail if no volume name provided", func() {
			removeResponse := localDriver.Remove(env, voldriver.RemoveRequest{
				Name: "",
			})
			Expect(removeResponse.Err).To(Equal("Missing mandatory 'volume_name'"))
		})

		It("should fail if no volume was created", func() {
			removeResponse := localDriver.Remove(env, voldriver.RemoveRequest{
				Name: volumeId,
			})
			Expect(removeResponse.Err).To(Equal("Volume '" + volumeId + "' not found"))
		})

		Context("when the volume has been created", func() {
			BeforeEach(func() {
				createSuccessful(env, localDriver, volumeId)
			})

			It("/VolumePlugin.Remove destroys volume", func() {
				removeResponse := localDriver.Remove(env, voldriver.RemoveRequest{
					Name: volumeId,
				})
				Expect(removeResponse.Err).To(Equal(""))
			})

			Context("when volume has been mounted", func() {
				It("/VolumePlugin.Remove unmounts and destroys volume", func() {
					mountSuccessful(env, localDriver, volumeId)

					removeResponse := localDriver.Remove(env, voldriver.RemoveRequest{
						Name: volumeId,
					})
					Expect(removeResponse.Err).To(Equal(""))

					getUnsuccessful(env, localDriver, volumeId)
				})
			})
		})

		Context("when the volume has not been created", func() {
			It("returns an error", func() {
				removeResponse := localDriver.Remove(env, voldriver.RemoveRequest{
					Name: volumeId,
				})
				Expect(removeResponse.Err).To(Equal("Volume '" + volumeId + "' not found"))
			})
		})
	})
})

func getUnsuccessful(env voldriver.Env, localDriver voldriver.Driver, volumeName string) {
	getResponse := localDriver.Get(env, voldriver.GetRequest{
		Name: volumeName,
	})

	Expect(getResponse.Err).To(Equal("Volume not found"))
	Expect(getResponse.Volume.Name).To(Equal(""))
}

func getSuccessful(env voldriver.Env, localDriver voldriver.Driver, volumeName string) voldriver.GetResponse {
	getResponse := localDriver.Get(env, voldriver.GetRequest{
		Name: volumeName,
	})

	Expect(getResponse.Err).To(Equal(""))
	Expect(getResponse.Volume.Name).To(Equal(volumeName))
	return getResponse
}

func createSuccessful(env voldriver.Env, localDriver voldriver.Driver, volumeName string) {
	createResponse := localDriver.Create(env, voldriver.CreateRequest{
		Name: volumeName,
	})
	Expect(createResponse.Err).To(Equal(""))
}

func mountSuccessful(env voldriver.Env, localDriver voldriver.Driver, volumeName string) {
	mountResponse := localDriver.Mount(env, voldriver.MountRequest{
		Name: volumeName,
	})
	Expect(mountResponse.Err).To(Equal(""))
}

func unmountSuccessful(env voldriver.Env, localDriver voldriver.Driver, volumeName string) {
	unmountResponse := localDriver.Unmount(env, voldriver.UnmountRequest{
		Name: volumeName,
	})
	Expect(unmountResponse.Err).To(Equal(""))
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}
