package git

import (
	"api/structs"

	"code.gitea.io/sdk/gitea"
)

func Connect(env *structs.Environment) (*gitea.Client, error) {
	return gitea.NewClient(env.GitUrl, gitea.SetToken(env.GitToken), gitea.SetGiteaVersion("1.19.9"))
}
