package packagerevision

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitDefaults(t *testing.T) {
	r := &PackageRevisionReconciler{}
	r.InitDefaults()

	assert.Equal(t, defaultMaxConcurrentReconciles, r.MaxConcurrentReconciles)
}

func TestBindFlags(t *testing.T) {
	r := &PackageRevisionReconciler{}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)

	r.BindFlags("pr-", flags)

	err := flags.Parse([]string{
		"--pr-max-concurrent-reconciles=10",
	})
	require.NoError(t, err)

	assert.Equal(t, 10, r.MaxConcurrentReconciles)
}

func TestBindFlagsDefaults(t *testing.T) {
	r := &PackageRevisionReconciler{}
	flags := flag.NewFlagSet("test", flag.ContinueOnError)

	r.BindFlags("pr-", flags)
	require.NoError(t, flags.Parse([]string{}))

	assert.Equal(t, defaultMaxConcurrentReconciles, r.MaxConcurrentReconciles)
}
