package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadLogConfig(t *testing.T) {
	var log Log
	os.Setenv("IMENV_LOG_REMAINLOGLEVEL", "5")
	err := Load("../../../config/", "log.yml", "IMENV_LOG", &log)
	assert.Nil(t, err)
	t.Log(log.RemainLogLevel)
	// assert.Equal(t, "../../../../logs/", log.StorageLocation)
}

func TestLoadMongoConfig(t *testing.T) {
	var mongo Mongo
	// os.Setenv("DEPLOYMENT_TYPE", "kubernetes")
	os.Setenv("IMENV_MONGODB_PASSWORD", "openIM1231231")
	// os.Setenv("IMENV_MONGODB_URI", "openIM123")
	// os.Setenv("IMENV_MONGODB_USERNAME", "openIM123")
	err := Load("../../../config/", "mongodb.yml", "IMENV_MONGODB", &mongo)
	// err := LoadApiConfig("../../../config/mongodb.yml", "IMENV_MONGODB", &mongo)

	assert.Nil(t, err)
	t.Log(mongo.Password)
	// assert.Equal(t, "openIM123", mongo.Password)
	t.Log(os.Getenv("IMENV_MONGODB_PASSWORD"))
	t.Log(mongo)
	// //export IMENV_OPENIM_RPC_USER_RPC_LISTENIP="0.0.0.0"
	// assert.Equal(t, "0.0.0.0", user.RPC.ListenIP)
	// //export IMENV_OPENIM_RPC_USER_RPC_PORTS="10110,10111,10112"
	// assert.Equal(t, []int{10110, 10111, 10112}, user.RPC.Ports)
}

func TestLoadMinioConfig(t *testing.T) {
	var storageConfig Minio
	err := Load("../../../config/", "minio.yml", "IMENV_MINIO", &storageConfig)
	assert.Nil(t, err)
	assert.Equal(t, "openim", storageConfig.Bucket)
}

func TestLoadWebhooksConfig(t *testing.T) {
	var webhooks Webhooks
	err := Load("../../../config/", "webhooks.yml", "IMENV_WEBHOOKS", &webhooks)
	assert.Nil(t, err)
	assert.Equal(t, 5, webhooks.BeforeAddBlack.Timeout)

}

func TestLoadOpenIMRpcUserConfig(t *testing.T) {
	var user User
	err := Load("../../../config/", "openim-rpc-user.yml", "IMENV_OPENIM_RPC_USER", &user)
	assert.Nil(t, err)
	assert.Equal(t, "0.0.0.0", user.RPC.ListenIP)
	assert.True(t, user.RPC.AutoSetPorts)
	assert.Nil(t, user.RPC.Ports)
}

func TestLoadNotificationConfig(t *testing.T) {
	var noti Notification
	err := Load("../../../config/", "notification.yml", "IMENV_NOTIFICATION", &noti)
	assert.Nil(t, err)
	assert.Equal(t, "Your friend's profile has been changed", noti.FriendRemarkSet.OfflinePush.Title)
}

func TestLoadOpenIMThirdConfig(t *testing.T) {
	var third Third
	err := Load("../../../config/", "openim-rpc-third.yml", "IMENV_OPENIM_RPC_THIRD", &third)
	assert.Nil(t, err)
	assert.Equal(t, "minio", third.Object.Enable)
	assert.Equal(t, "https://oss-cn-chengdu.aliyuncs.com", third.Object.Oss.Endpoint)
	assert.Equal(t, "demo-9999999", third.Object.Oss.Bucket)
	assert.Equal(t, "https://demo-9999999.oss-cn-chengdu.aliyuncs.com", third.Object.Oss.BucketURL)
	assert.Equal(t, "", third.Object.Oss.AccessKeyID)
	assert.Equal(t, "", third.Object.Oss.AccessKeySecret)
	assert.Equal(t, "", third.Object.Oss.SessionToken)
	assert.Equal(t, false, third.Object.Oss.PublicRead)
}

func TestTransferConfig(t *testing.T) {
	var tran MsgTransfer
	err := Load("../../../config/", "openim-msgtransfer.yml", "IMENV_OPENIM-MSGTRANSFER", &tran)
	assert.Nil(t, err)
	assert.Equal(t, true, tran.Prometheus.Enable)
	assert.Equal(t, true, tran.Prometheus.AutoSetPorts)
}
