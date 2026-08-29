package command

import (
	"net/http"
	"testing"

	"github.com/oapi-codegen/nullable"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	dp "github.com/stellwerk-labs/platform-orchestrator-cli/clients/platform-orchestrator-dp"
	"github.com/stellwerk-labs/platform-orchestrator-cli/internal/ref"
)

const (
	metadataKeyCommandName = "metadata-key"
	metadataKeyOwner       = "owner"
)

func TestCreateMetadataKey(t *testing.T) {
	orgID, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	body := dp.MetadataKeyCreateBody{
		Name: "cost-center", Description: ref.Ref("Billing code"), Schema: dp.MetadataKeySchema{Type: dp.MetadataKeySchemaTypeString},
	}
	dpc.EXPECT().CreateMetadataKeyWithResponse(gomock.Any(), orgID, body).Return(&dp.CreateMetadataKeyResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusCreated},
		JSON201:      &dp.MetadataKey{Name: body.Name, Description: body.Description, Schema: body.Schema},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testCreateCmd, metadataKeyCommandName, "cost-center", `--set-json={"description":"Billing code","schema":{"type":"string"}}`})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "cost-center")
		assert.Contains(t, stdout, "Billing code")
	}
}

func TestListMetadataKeysCollectsPages(t *testing.T) {
	orgID, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	dpc.EXPECT().ListMetadataKeysWithResponse(gomock.Any(), orgID, &dp.ListMetadataKeysParams{}).Return(&dp.ListMetadataKeysResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.MetadataKeyPage{NextPageToken: ref.Ref("next")},
	}, nil)
	dpc.EXPECT().ListMetadataKeysWithResponse(gomock.Any(), orgID, &dp.ListMetadataKeysParams{Page: ref.Ref("next")}).Return(&dp.ListMetadataKeysResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.MetadataKeyPage{Items: []dp.MetadataKey{{Name: metadataKeyOwner, Schema: dp.MetadataKeySchema{Type: dp.MetadataKeySchemaTypeString}}}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testGetCmd, "metadata-keys"})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, metadataKeyOwner)
	}
}

func TestUpdateMetadataKey(t *testing.T) {
	orgID, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	description := "Updated"
	body := dp.MetadataKeyUpdateBody{Description: nullable.NewNullableWithValue(description)}
	dpc.EXPECT().UpdateMetadataKeyWithResponse(gomock.Any(), orgID, metadataKeyOwner, body).Return(&dp.UpdateMetadataKeyResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.MetadataKey{Name: metadataKeyOwner, Description: &description, Schema: dp.MetadataKeySchema{Type: dp.MetadataKeySchemaTypeString}},
	}, nil)

	stdout, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, outFlag, jsonOutput, testUpdateCmd, metadataKeyCommandName, metadataKeyOwner, `--set=description=Updated`})
	if assert.NoError(t, err) {
		assert.Contains(t, stdout, "Updated")
	}
}

func TestUpdateMetadataKeyClearsOptionalFields(t *testing.T) {
	orgID, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	body := dp.MetadataKeyUpdateBody{
		Description: nullable.NewNullNullable[string](),
		Schema: &dp.UpdateMetadataKeySchema{
			Format:  nullable.NewNullNullable[string](),
			Pattern: nullable.NewNullNullable[string](),
		},
	}
	dpc.EXPECT().UpdateMetadataKeyWithResponse(gomock.Any(), orgID, metadataKeyOwner, body).Return(&dp.UpdateMetadataKeyResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &dp.MetadataKey{Name: metadataKeyOwner, Schema: dp.MetadataKeySchema{Type: dp.MetadataKeySchemaTypeString}},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{
		orgFlag, orgID, outFlag, jsonOutput, testUpdateCmd, metadataKeyCommandName, metadataKeyOwner,
		`--set-json={"description":null,"schema":{"format":null,"pattern":null}}`,
	})
	assert.NoError(t, err)
}

func TestDeleteMetadataKeyNotFound(t *testing.T) {
	orgID, _, dpc, ctx, fin := setupTestContext(t)
	defer fin()
	dpc.EXPECT().DeleteMetadataKeyWithResponse(gomock.Any(), orgID, "missing").Return(&dp.DeleteMetadataKeyResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
		JSON404:      &dp.N404NotFound{Message: envTypesTestNotFound},
	}, nil)

	_, _, err := executeAndResetCommand(ctx, RootCmd, []string{orgFlag, orgID, testDeleteCmd, metadataKeyCommandName, "missing"})
	assert.EqualError(t, err, `metadata key "missing" not found in organization '`+orgID+`'`)
}
