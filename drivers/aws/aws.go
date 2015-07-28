package ebs

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/boltdb/bolt"
	"github.com/libopenstorage/openstorage/api"
	"github.com/libopenstorage/openstorage/volume"
)

const (
	Name          = "aws"
	AwsBucketName = "OpenStorageAWSBucket"
)

var (
	devMinor int32
)

type awsVolume struct {
	device     string
	instanceID string
}

// Implements the open storage volume interface.
type awsProvider struct {
	db  *bolt.DB
	ec2 *ec2.EC2
}

func Init(params volume.DriverParams) (volume.VolumeDriver, error) {
	// Initialize the EC2 interface.
	creds := credentials.NewEnvCredentials()
	inst := &awsProvider{ec2: ec2.New(&aws.Config{
		Region:      "us-west-1",
		Credentials: creds,
	}),
	}

	// Create a DB if one does not exist.  This is where we persist the
	// Amazon instance ID, sdevice and volume ID mappings.
	db, err := bolt.Open("openstorage.aws.db", 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucket([]byte(AwsBucketName))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})

	inst.db = db

	return inst, err
}

// AWS provisioned IOPS range is 100 - 20000.
func mapIops(cos api.VolumeCos) int64 {
	if cos < 3 {
		return 1000
	} else if cos < 7 {
		return 10000
	} else {
		return 20000
	}
}

func (self *awsProvider) get(volumeID string) (*awsVolume, error) {
	v := &awsVolume{}

	err := self.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(AwsBucketName))
		b := bucket.Get([]byte(volumeID))

		if b == nil {
			return errors.New("no such volume ID")
		} else {
			err := json.Unmarshal(b, v)
			return err
		}
	})

	return v, err
}

func (self *awsProvider) put(volumeID string, v *awsVolume) error {
	b, _ := json.Marshal(v)

	err := self.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(AwsBucketName))
		err := bucket.Put([]byte(volumeID), b)
		return err
	})

	return err
}

func (self *awsProvider) String() string {
	return Name
}

func (self *awsProvider) Create(l api.VolumeLocator, opt *api.CreateOptions, spec *api.VolumeSpec) (api.VolumeID, error) {
	availabilityZone := "us-west-1a"
	sz := int64(spec.Size / (1024 * 1024 * 1024))
	iops := mapIops(spec.Cos)
	req := &ec2.CreateVolumeInput{
		AvailabilityZone: &availabilityZone,
		Size:             &sz,
		IOPS:             &iops}
	v, err := self.ec2.CreateVolume(req)

	return api.VolumeID(*v.VolumeID), err
}

func (self *awsProvider) Attach(volInfo api.VolumeID) (string, error) {
	devMinor++
	device := fmt.Sprintf("/dev/ec2%v", int(devMinor))
	volumeID := string(volInfo)
	instanceID := string("")
	req := &ec2.AttachVolumeInput{
		Device:     &device,
		InstanceID: &instanceID,
		VolumeID:   &volumeID,
	}

	resp, err := self.ec2.AttachVolume(req)
	if err != nil {
		return "", err
	}

	err = self.put(volumeID, &awsVolume{instanceID: instanceID})

	return *resp.Device, err
}

func (self *awsProvider) Mount(volumeID api.VolumeID, mountpath string) error {
	return nil
}

func (self *awsProvider) Detach(volID api.VolumeID) error {
	return nil
}

func (self *awsProvider) Unmount(volumeID api.VolumeID, mountpath string) error {
	return nil
}

func (self *awsProvider) Delete(volID api.VolumeID) error {
	return nil
}

func (self *awsProvider) Format(id api.VolumeID) error {
	return nil
}

func (self *awsProvider) Inspect(ids []api.VolumeID) ([]api.Volume, error) {
	return nil, nil
}

func (self *awsProvider) Enumerate(locator api.VolumeLocator, labels api.Labels) []api.Volume {
	return nil
}

func (self *awsProvider) Snapshot(volID api.VolumeID, labels api.Labels) (snap api.SnapID, err error) {
	return "", nil
}

func (self *awsProvider) SnapDelete(snapID api.SnapID) (err error) {
	return nil
}

func (self *awsProvider) SnapInspect(snapID api.SnapID) (snap api.VolumeSnap, err error) {
	return api.VolumeSnap{}, nil
}

func (self *awsProvider) SnapEnumerate(locator api.VolumeLocator, labels api.Labels) *[]api.SnapID {
	return nil
}

func (self *awsProvider) Stats(volID api.VolumeID) (stats api.VolumeStats, err error) {
	return api.VolumeStats{}, nil
}

func (self *awsProvider) Alerts(volID api.VolumeID) (stats api.VolumeAlerts, err error) {
	return api.VolumeAlerts{}, nil
}

func (self *awsProvider) Shutdown() {
	fmt.Printf("%s Shutting down", Name)
}

func init() {
	// Register ourselves as an openstorage volume driver.
	volume.Register(Name, Init)
}
