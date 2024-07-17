package listcmd_test

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestTableRendererTTY(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	io.SetStdoutTTY(true)
	r := listcmd.TableRenderer{
		IO: io,
	}

	err := r.Render([]listcmd.Sponsor{"sponsor1", "sponsor2"})
	require.NoError(t, err)

	expectedOutput := heredoc.Doc(`
	SPONSOR
	sponsor1
	sponsor2
	`)
	require.Equal(t, expectedOutput, stdout.String())
}
