package route

import "testing"

func TestNormalizeInstallRootPathByOS(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		isWindows bool
		want      string
	}{
		{
			name:      "windows_unc_path_preserved",
			input:     `\\imb-nas\repositories`,
			isWindows: true,
			want:      `\\imb-nas\repositories`,
		},
		{
			name:      "windows_drive_path_preserved",
			input:     `C:\gogs\data`,
			isWindows: true,
			want:      `C:\gogs\data`,
		},
		{
			name:      "non_windows_backslashes_replaced",
			input:     `\\imb-nas\repositories`,
			isWindows: false,
			want:      `//imb-nas/repositories`,
		},
		{
			name:      "non_windows_forward_slashes_unchanged",
			input:     `/var/lib/gogs/repos`,
			isWindows: false,
			want:      `/var/lib/gogs/repos`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeInstallRootPathByOS(tc.input, tc.isWindows)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
