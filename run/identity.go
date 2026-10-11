package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/liang-chang/debounce/types"
)

// GenerateTaskID separates explicit keys from command identities and preserves
// raw Unix argument bytes via JSON's base64 encoding for byte slices.
func GenerateTaskID(args *types.DebounceCommand) (string, error) {
	identity := struct {
		Version   int
		Kind      string
		Key       []byte
		Command   [][]byte
		Directory []byte
	}{Version: 2}
	if args.Key != "" {
		identity.Kind, identity.Key = "explicit", []byte(args.Key)
	} else {
		identity.Kind, identity.Command = "command", commandBytes(args.Command)
		if args.Local {
			cwd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			cwd, err = filepath.EvalSymlinks(cwd)
			if err != nil {
				return "", err
			}
			identity.Directory = []byte(filepath.Clean(cwd))
		}
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func commandBytes(args []string) [][]byte {
	values := make([][]byte, len(args))
	for i, arg := range args {
		values[i] = []byte(arg)
	}
	return values
}

func scopedEndpoint(userID, task string) string {
	sum := sha256.Sum256([]byte(userID + ":" + task))
	return endpointName(hex.EncodeToString(sum[:]))
}
