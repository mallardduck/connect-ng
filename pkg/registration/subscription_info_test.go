package registration

import (
	"errors"
	"testing"

	"github.com/SUSE/connect-ng/pkg/connection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFetchSubscriptionInfoSuccess(t *testing.T) {
	assert := assert.New(t)

	conn, _ := connection.NewMockConnectionWithCredentials()

	payload := fixture(t, "pkg/registration/subscription_info.json")
	conn.On("Do", mock.Anything).Return(payload, nil).Run(
		checkAuthByRegcode(t, "test-regcode"),
	)

	info, err := FetchSubscriptionInfo(conn, "test-regcode")
	assert.NoError(err)
	assert.NotNil(info)
	assert.Equal("full", info.Kind)
	assert.Equal("SUSE Linux Enterprise Server, x86-64, 1 Virtual Machine, Standard Subscription, 1 Year", info.Name)
	assert.Equal(20, info.Limit)
	assert.Equal("silent", info.Notifications)
	assert.Len(info.ProductClasses, 2)
	assert.Equal("RANCHER-X86", info.ProductClasses[0].Name)
	assert.Equal("Rancher Manager offers a comprehensive suite of products built on a single code base.", info.ProductClasses[0].Description)
	assert.Equal("SLES-X86", info.ProductClasses[1].Name)
}

func TestFetchSubscriptionInfoAPIError(t *testing.T) {
	assert := assert.New(t)

	conn, _ := connection.NewMockConnectionWithCredentials()
	conn.On("Do", mock.Anything).Return([]byte{}, errors.New("Invalid regcode"))

	_, err := FetchSubscriptionInfo(conn, "invalid-regcode")
	assert.Error(err)
}

func TestFetchSubscriptionInfoMalformedJSON(t *testing.T) {
	assert := assert.New(t)

	conn, _ := connection.NewMockConnectionWithCredentials()
	conn.On("Do", mock.Anything).Return([]byte("{invalid json}"), nil)

	_, err := FetchSubscriptionInfo(conn, "test-regcode")
	assert.Error(err)
}

func TestFetchSubscriptionProductsSuccess(t *testing.T) {
	assert := assert.New(t)

	conn, _ := connection.NewMockConnectionWithCredentials()

	payload := fixture(t, "pkg/registration/subscription_products.json")
	conn.On("Do", mock.Anything).Return(payload, nil).Run(
		checkAuthByRegcode(t, "test-regcode"),
	)

	products, err := FetchSubscriptionProducts(conn, "test-regcode")
	assert.NoError(err)
	assert.Len(products, 2)
	assert.Equal("SLES", products[0].Identifier)
	assert.Equal("SUSE Linux Enterprise Server", products[0].Name)
	assert.Equal("x86_64", products[0].Arch)
	assert.Equal("rancher-manager", products[1].Identifier)
	assert.Equal("SUSE Rancher Manager", products[1].Name)
}

func TestFetchSubscriptionProductsAPIError(t *testing.T) {
	assert := assert.New(t)

	conn, _ := connection.NewMockConnectionWithCredentials()
	conn.On("Do", mock.Anything).Return([]byte{}, errors.New("Invalid regcode"))

	_, err := FetchSubscriptionProducts(conn, "invalid-regcode")
	assert.Error(err)
}

func TestFetchSubscriptionProductsMalformedJSON(t *testing.T) {
	assert := assert.New(t)

	conn, _ := connection.NewMockConnectionWithCredentials()
	conn.On("Do", mock.Anything).Return([]byte("[{invalid json}]"), nil)

	_, err := FetchSubscriptionProducts(conn, "test-regcode")
	assert.Error(err)
}
