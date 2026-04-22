package cron

import (
	"context"
	"os"
	"testing"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	kdisc "github.com/openimsdk/open-im-server/v3/pkg/common/discovery"
	pbconversation "github.com/openimsdk/protocol/conversation"
	"github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/protocol/third"
	"github.com/openimsdk/tools/mcontext"
	"github.com/openimsdk/tools/mw"
	"github.com/robfig/cron/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestName(t *testing.T) {
	if os.Getenv("OPENIM_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENIM_INTEGRATION_TEST=1 with local etcd and RPCs (e.g. docker compose + mage) to run")
	}
	conf := &config.Discovery{
		Enable: config.ETCD,
		Etcd: config.Etcd{
			RootDirectory: "openim",
			Address:       []string{"localhost:12379"},
		},
	}
	client, err := kdisc.NewDiscoveryRegister(conf, nil)
	if err != nil {
		panic(err)
	}
	client.AddOption(mw.GrpcClient(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	ctx := mcontext.SetOpUserID(context.Background(), "imAdmin")
	msgConn, err := client.GetConn(ctx, "msg-rpc-service")
	if err != nil {
		panic(err)
	}
	thirdConn, err := client.GetConn(ctx, "third-rpc-service")
	if err != nil {
		panic(err)
	}

	conversationConn, err := client.GetConn(ctx, "conversation-rpc-service")
	if err != nil {
		panic(err)
	}

	srv := &cronServer{
		ctx: ctx,
		config: &Config{
			CronTask: config.CronTask{
				RetainChatRecords: 1,
				FileExpireTime:    1,
				DeleteObjectType:  []string{"msg-picture", "msg-file", "msg-voice", "msg-video", "msg-video-snapshot", "sdklog", ""},
			},
		},
		cron:               cron.New(),
		msgClient:          msg.NewMsgClient(msgConn),
		conversationClient: pbconversation.NewConversationClient(conversationConn),
		thirdClient:        third.NewThirdClient(thirdConn),
	}
	srv.deleteMsg()
	//srv.clearS3()
	//srv.clearUserMsg()
}
