package combat

// randomRange e o RandomRange(a, b) do Delphi: sorteia em [a, b).
func randomRange(rnd Rand, a, b int) int { return a + rnd.IntN(b-a) }

// Roll3 sorteia o tipo de dano de um golpe. Porte de TBaseMob.GetDamageType3,
// o sorteio que SendDamage e as skills de todas as classes usam na origem
// (GetDamageType, o da roleta de 1000 posicoes, nao e chamado por elas).
//
// Em vez de uma roleta, cada etapa compara o atributo do atacante com o do
// alvo:
//
//  1. Acerto x Esquiva. Contra mob o Acerto ganha 20..39. Com a diferenca
//     (Acerto - Esquiva) >= 0 o golpe erra se sorteio + min(diferenca, 10) <= 20
//     e Esquiva >= 7; com a diferenca negativa, se sorteio <= 30 + min(|dif|, 10)
//     e Esquiva >= 3. Dois erros seguidos garantem o terceiro acerto.
//  2. Critico x Resistencia: (CritRes+10)*1,4. Com a diferenca >= 0 critica se
//     sorteio + min(dif, 25) >= 40 e Critico >= 5; negativa, se sorteio <=
//     15 - min(|dif|, 5) e Critico >= 9. Contra mob, Critico >= 100 ainda
//     critica em um de cada tres golpes.
//  3. ForceCrit (buff 6347) vira o golpe em critico.
//  4. So o ataque basico (skill 0) que nao critou tenta duplo, comparando
//     DuploAtk com (DuploRes+5)*1,5.
//
// O sorteio e RandomRange(1, 101), isto e, de 1 a 100.
func (r *Roller) Roll3(a Attacker, d Defender, skill uint32, rnd Rand) DamageType {
	res := Normal

	// ---- acerto x esquiva
	acerto := int(a.Acerto)
	esquiva := int(d.Esquiva)
	if !d.IsPlayer {
		acerto += randomRange(rnd, 20, 40)
	}
	taxa := acerto - esquiva
	errou := false
	if taxa >= 0 {
		sorteio := randomRange(rnd, 1, 101)
		taxa = min(taxa, 10)
		errou = sorteio+taxa <= 20 && esquiva >= 7
	} else {
		taxa = min(-taxa, 10)
		sorteio := randomRange(rnd, 1, 101)
		errou = sorteio <= 30+taxa && esquiva >= 3
	}
	if errou {
		r.missCount++
		if r.missCount < 3 {
			return Miss
		}
		r.missCount = 0
	}

	// ---- critico x resistencia
	critico := int(a.Critical)
	resCrit := int(float64(int(d.CritRes)+10) * 1.4)
	taxaCrit := critico - resCrit
	if taxaCrit >= 0 {
		sorteio := randomRange(rnd, 1, 101)
		taxaCrit = min(taxaCrit, 25)
		if sorteio+taxaCrit >= 40 && critico >= 5 {
			res = Critical
		}
		if !d.IsPlayer && critico >= 100 && randomRange(rnd, 1, 4) == 2 {
			res = Critical
		}
	} else {
		taxaCrit = min(-taxaCrit, 5)
		sorteio := randomRange(rnd, 1, 101)
		if sorteio <= 15-taxaCrit && critico >= 9 {
			res = Critical
		}
	}
	if a.ForceCrit {
		res = Critical
	}

	// ---- duplo (so no ataque basico que nao critou)
	if res == Normal && skill == 0 {
		duplo := int(a.DuploAtk)
		resDuplo := int(float64(int(d.DuploRes)+5) * 1.5)
		taxaDuplo := duplo - resDuplo
		sorteio := randomRange(rnd, 1, 101)
		if taxaDuplo >= 0 {
			taxaDuplo = min(taxaDuplo, 10)
			if sorteio >= 80-taxaDuplo && duplo >= 3 {
				res = Double
			}
		} else {
			taxaDuplo = min(-taxaDuplo, 10)
			if sorteio <= 15+taxaDuplo && duplo >= 5 {
				res = Double
			}
		}
	}
	return res
}
