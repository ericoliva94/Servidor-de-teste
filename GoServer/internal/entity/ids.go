package entity

// Faixas de ClientID usadas pelo servidor. Vem dos "case target.ClientID of"
// de AddTargetToList e dos limites dos vetores globais em ServerSocket.pas.
//
// A disciplina importa: Players e ARRAY [1..1000], entao indexar com 0 ou com
// um id de mob le memoria fora do vetor. O codigo original testava so
// "ClientID <= MAX_CONNECTIONS" em varios pontos (MOB.pas, PET.pas, NPC.pas,
// BaseMob.pas, Dungeon.pas), sem o limite de baixo, e Players[0] cai antes do
// inicio do vetor.
const (
	// MinPlayerID e o primeiro indice valido de Servers[].Players.
	MinPlayerID = 1
	// MaxPlayerIDHardLimit e o tamanho do vetor Players (ARRAY [1..1000]).
	// MAX_CONNECTIONS vem do .ini e nao pode passar disso.
	MaxPlayerIDHardLimit = 1000

	// Faixas de mob, conforme o case de AddTargetToList.
	MinMobID     = 1001
	MaxMobID     = 3339
	MinMobID2    = 3370
	MaxMobID2    = 9147
	MinDungeonID = 9148
)

// TargetType marca o tipo de alvo guardado em TMobTarget.TargetType.
const (
	TargetPlayer uint8 = 0
	TargetMob    uint8 = 1
)

// IsPlayerID diz se o id cai na faixa valida de Servers[].Players, dado o
// MAX_CONNECTIONS em uso.
//
// Use sempre isto antes de indexar a tabela de jogadores: e o teste que
// faltava nos "if (i <= MAX_CONNECTIONS)" do original.
func IsPlayerID(id uint16, maxConnections uint16) bool {
	limit := maxConnections
	if limit > MaxPlayerIDHardLimit {
		limit = MaxPlayerIDHardLimit
	}
	return id >= MinPlayerID && id <= limit
}

// IsMobID diz se o id cai numa das faixas de mob.
func IsMobID(id uint16) bool {
	return (id >= MinMobID && id <= MaxMobID) || (id >= MinMobID2 && id <= MaxMobID2)
}

// IsDungeonMobID diz se o id e de mob de instancia de dungeon.
func IsDungeonMobID(id uint16) bool { return id >= MinDungeonID }

// ClampMaxConnections limita o MAX_CONNECTIONS lido do .ini ao tamanho real do
// vetor Players.
//
// No original MAX_CONNECTIONS vinha de StrToInt(Users) sem limite nenhum, e
// todo "for i := 1 to MAX_CONNECTIONS do ... Players[i]" passaria do fim do
// vetor se o .ini trouxesse um valor acima de 1000.
func ClampMaxConnections(v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > MaxPlayerIDHardLimit {
		return MaxPlayerIDHardLimit
	}
	return uint16(v)
}
