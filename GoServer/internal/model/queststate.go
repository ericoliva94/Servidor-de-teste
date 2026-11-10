package model

// QuestProgress e o estado de uma quest de um personagem (QuestDin de
// FilesData.pas, sem a copia da definicao: ela vem do questdata).
type QuestProgress struct {
	QuestID uint32
	Done    bool
	// Complete conta o progresso de cada um dos 5 requisitos.
	Complete  [5]uint8
	UpdatedAt int64 // unixtime
	CreatedAt int64 // unixtime
}
