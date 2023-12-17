package git

import (
	"runik-api/structs"

	"code.gitea.io/sdk/gitea"
)

func Connect(env *structs.Environment) (*gitea.Client, error) {
	return gitea.NewClient(env.GitUrl, gitea.SetToken(env.GitToken))
}
