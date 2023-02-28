package wallet

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/venus/venus-shared/api"
	"github.com/filecoin-project/venus/venus-shared/types"
)

type ICommon interface {
	// Auth
	AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) //perm:read
	AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error)    //perm:admin

	LogList(context.Context) ([]string, error)         //perm:read
	LogSetLevel(context.Context, string, string) error //perm:write

	ListSignedRecord(ctx context.Context, param *types.QuerySignRecordParams) ([]types.SignRecord, error) //perm:read

	api.Version
}
