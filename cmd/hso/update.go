package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hijoushoku7/hijo-server-ops/internal/msg"
)

var (
	versionHTTPClient = &http.Client{Timeout: 3 * time.Second}
	updateHTTPClient  = &http.Client{Timeout: 5 * time.Minute}
	executablePath    = os.Executable
)

func runVersion(output io.Writer) error {
	if _, err := fmt.Fprintln(output, msg.VersionOutput(version, msg.Lang, runtime.GOARCH)); err != nil {
		return err
	}

	latest, err := latestRelease(versionHTTPClient)
	if err != nil || latest.Tag == version {
		return nil
	}
	_, err = fmt.Fprintln(output, msg.UpdateAvailable(latest.Tag))
	return err
}

func runUpdate(output io.Writer) error {
	latest, err := latestRelease(updateHTTPClient)
	if err != nil {
		return err
	}
	if latest.Tag == version {
		_, err := fmt.Fprintln(output, msg.AlreadyLatest(version))
		return err
	}

	assetName := fmt.Sprintf("hso_%s_linux_%s_%s.tar.gz", latest.Tag, runtime.GOARCH, msg.Lang)
	archiveAsset, ok := latest.asset(assetName)
	if !ok {
		return msg.ReleaseAssetMissing(assetName)
	}
	checksumAsset, ok := latest.asset("checksums.txt")
	if !ok {
		return msg.ReleaseAssetMissing("checksums.txt")
	}
	if archiveAsset.URL == "" {
		return msg.ReleaseAssetURLMissing(assetName)
	}
	if checksumAsset.URL == "" {
		return msg.ReleaseAssetURLMissing("checksums.txt")
	}

	executable, err := executablePath()
	if err != nil {
		return msg.ExecutablePathFailed(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return msg.ExecutablePathFailed(err)
	}
	privileged, err := replacementPrivilege(executable)
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "hso-update-")
	if err != nil {
		return msg.UpdateTemporaryDirectoryFailed(err)
	}
	defer os.RemoveAll(work)

	archivePath := filepath.Join(work, assetName)
	checksumsPath := filepath.Join(work, "checksums.txt")
	if err := downloadAsset(updateHTTPClient, archiveAsset, archivePath); err != nil {
		return err
	}
	if err := downloadAsset(updateHTTPClient, checksumAsset, checksumsPath); err != nil {
		return err
	}
	if err := verifyChecksumFile(checksumsPath, archivePath, assetName); err != nil {
		return err
	}

	binaryPath := filepath.Join(work, "hso")
	member := fmt.Sprintf("hso_%s_linux_%s_%s/hso", latest.Tag, runtime.GOARCH, msg.Lang)
	if err := extractBinary(archivePath, binaryPath, member); err != nil {
		return err
	}
	if err := replaceExecutable(binaryPath, executable, privileged); err != nil {
		return msg.ReplaceExecutableFailed(executable, err)
	}

	_, err = fmt.Fprintln(output, msg.UpdateComplete(latest.Tag, executable))
	return err
}

func downloadAsset(client *http.Client, asset releaseAsset, destination string) error {
	request, err := githubRequest(asset.URL)
	if err != nil {
		return msg.DownloadAssetFailed(asset.Name, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return msg.DownloadAssetFailed(asset.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return msg.DownloadAssetStatus(asset.Name, response.Status)
	}

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return msg.DownloadAssetFailed(asset.Name, err)
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return msg.DownloadAssetFailed(asset.Name, copyErr)
	}
	if closeErr != nil {
		return msg.DownloadAssetFailed(asset.Name, closeErr)
	}
	return nil
}

func verifyChecksumFile(checksumsPath, archivePath, assetName string) error {
	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		return msg.ReadChecksumsFailed(err)
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return msg.ReadArchiveFailed(assetName, err)
	}
	defer archive.Close()
	return verifyChecksum(checksums, archive, assetName)
}

func verifyChecksum(checksums []byte, archive io.Reader, assetName string) error {
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[len(fields)-1], "*") != assetName {
			continue
		}
		decoded, err := hex.DecodeString(fields[0])
		if err == nil && len(decoded) == sha256.Size {
			expected = fields[0]
		}
		break
	}
	if expected == "" {
		return msg.ChecksumNotFound(assetName)
	}

	digest := sha256.New()
	if _, err := io.Copy(digest, archive); err != nil {
		return msg.CalculateChecksumFailed(assetName, err)
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return msg.ChecksumMismatch(assetName)
	}
	return nil
}

func extractBinary(archivePath, destination, member string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return msg.ExtractArchiveFailed(err)
	}
	defer archive.Close()
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return msg.ExtractArchiveFailed(err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return msg.BinaryMissingFromArchive()
		}
		if err != nil {
			return msg.ExtractArchiveFailed(err)
		}
		if header.Name != member {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return msg.BinaryMissingFromArchive()
		}

		binary, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return msg.ExtractArchiveFailed(err)
		}
		_, copyErr := io.Copy(binary, tarReader)
		closeErr := binary.Close()
		if copyErr != nil {
			return msg.ExtractArchiveFailed(copyErr)
		}
		if closeErr != nil {
			return msg.ExtractArchiveFailed(closeErr)
		}
		return nil
	}
}

func replacementPrivilege(target string) (string, error) {
	if targetDirectoryAccess(target) == nil {
		return "", nil
	}
	if os.Geteuid() == 0 {
		return "", msg.ReplaceExecutableFailed(target, syscall.EACCES)
	}
	return authenticatePrivilege(target, isTerminal(os.Stderr), os.Stderr)
}

func authenticatePrivilege(target string, interactive bool, notice io.Writer) (string, error) {
	for _, name := range []string{"sudo", "doas"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		args := []string{"-n", "true"}
		if interactive {
			// パスワードを聞かれる前に、なぜ root 権限が要るのかを出す。
			fmt.Fprintln(notice, msg.PrivilegeExplanation(target, name))
			args = []string{"true"}
			if name == "sudo" {
				args = []string{"-v"}
			}
		}
		command := exec.Command(path, args...)
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return "", msg.PrivilegeAuthenticationFailed(name, err)
		}
		return path, nil
	}
	return "", msg.PrivilegeToolMissing(target)
}

func replaceExecutable(source, target, privileged string) error {
	if privileged == "" {
		return copyChmodRename(source, target)
	}

	// 昇格する範囲は、対象と同じディレクトリへの複製、権限設定、rename だけ。
	command := exec.Command(privileged, "sh", "-c", `
		staging=$(mktemp "$2/.$4-XXXXXX") || exit 1
		trap 'rm -f "$staging"' 0
		trap 'exit 1' 1 2 15
		cp "$1" "$staging" || exit $?
		chmod 0755 "$staging" || exit $?
		mv -f "$staging" "$3" || exit $?
		trap - 0 1 2 15
	`, "_", source, filepath.Dir(target), target, filepath.Base(target))
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func copyChmodRename(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-*")
	if err != nil {
		return err
	}
	staging := output.Name()
	renamed := false
	defer func() {
		if !renamed {
			output.Close()
			os.Remove(staging)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Chmod(0o755); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		return err
	}
	renamed = true
	return nil
}
