package lrpt

// Reed-Solomon (255,223) decoder with the Meteor-M / CCSDS parameters:
// GF(2^8) with field polynomial 0x187, generator roots β^112..β^143
// where β = α^11 (equivalent to the CCSDS dual-basis code without a
// representation change). Codewords are transmitted first-byte =
// highest-degree coefficient; four codewords are byte-interleaved over
// the 1020-byte CADU data zone.

const (
	rsFieldPoly = 0x187
	rsFirstRoot = 112
	rsRootSkip  = 11
	rsParity    = RSBlockLen - RSDataLen // 32
)

type rsCodec struct {
	exp [510]byte          // β^i (doubled to avoid mod in products)
	log [256]byte          // log_β
	gen [rsParity + 1]byte // generator polynomial (for the test encoder)
}

func newRSCodec() *rsCodec {
	r := &rsCodec{}

	// Build α tables for GF(2^8)/0x187, then re-index by β = α^11.
	var aexp [255]int
	v := 1
	for i := range 255 {
		aexp[i] = v
		v <<= 1
		if v&0x100 != 0 {
			v ^= rsFieldPoly
		}
	}
	for i := range 255 {
		b := aexp[(i*rsRootSkip)%255] // β^i
		r.exp[i] = byte(b)
		r.exp[i+255] = byte(b)
		r.log[b] = byte(i)
	}

	// Generator polynomial g(x) = Π_{i=112}^{143} (x − β^i)
	r.gen[0] = 1
	for i := range rsParity {
		root := r.exp[(rsFirstRoot+i)%255]
		// multiply gen by (x + root)
		var prev byte
		for j := 0; j <= i+1; j++ {
			cur := r.gen[j]
			r.gen[j] = prev ^ r.mul(cur, root)
			prev = cur
		}
	}
	return r
}

func (r *rsCodec) mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return r.exp[int(r.log[a])+int(r.log[b])]
}

func (r *rsCodec) div(a, b byte) byte {
	if a == 0 {
		return 0
	}
	return r.exp[int(r.log[a])+255-int(r.log[b])]
}

// encode computes the 32 parity bytes for 223 data bytes (tests only).
func (r *rsCodec) encode(data []byte) []byte {
	par := make([]byte, rsParity)
	for _, d := range data {
		f := d ^ par[0]
		copy(par, par[1:])
		par[rsParity-1] = 0
		if f != 0 {
			for j := range rsParity {
				par[j] ^= r.mul(f, r.gen[rsParity-1-j])
			}
		}
	}
	return par
}

// decode corrects a 255-byte codeword in place. Returns the number of
// corrected bytes, or -1 if uncorrectable.
func (r *rsCodec) decode(cw []byte) int {
	// Syndromes S_i = cw(β^(112+i)), Horner over stored order
	// (cw[0] = coefficient of x^254).
	var synd [rsParity]byte
	allZero := true
	for i := range rsParity {
		root := r.exp[(rsFirstRoot+i)%255]
		var s byte
		for _, c := range cw {
			s = r.mul(s, root) ^ c
		}
		synd[i] = s
		if s != 0 {
			allZero = false
		}
	}
	if allZero {
		return 0
	}

	// Berlekamp–Massey: find error locator λ(x)
	var lambda, prevLambda, tmp [rsParity + 1]byte
	lambda[0], prevLambda[0] = 1, 1
	var l int
	b := byte(1)
	m := 1
	for n := range rsParity {
		// discrepancy
		d := synd[n]
		for i := 1; i <= l; i++ {
			d ^= r.mul(lambda[i], synd[n-i])
		}
		if d == 0 {
			m++
			continue
		}
		if 2*l <= n {
			copy(tmp[:], lambda[:])
			coef := r.div(d, b)
			for i := 0; i+m <= rsParity; i++ {
				lambda[i+m] ^= r.mul(coef, prevLambda[i])
			}
			l = n + 1 - l
			copy(prevLambda[:], tmp[:])
			b = d
			m = 1
		} else {
			coef := r.div(d, b)
			for i := 0; i+m <= rsParity; i++ {
				lambda[i+m] ^= r.mul(coef, prevLambda[i])
			}
			m++
		}
	}
	if l > rsParity/2 {
		return -1
	}

	// Chien search: roots of λ. Error at coefficient index j (stored
	// order) corresponds to locator X = β^(254-j).
	var errPos [rsParity / 2]int
	nErr := 0
	for j := range RSBlockLen {
		// evaluate λ(β^-(254-j)) = λ(β^((j+1) mod 255))
		e := (j + 1) % 255
		var v byte
		for i := l; i >= 0; i-- {
			v = r.mul(v, r.exp[e]) ^ lambda[i]
		}
		if v == 0 {
			if nErr >= len(errPos) {
				return -1
			}
			errPos[nErr] = j
			nErr++
		}
	}
	if nErr != l {
		return -1
	}

	// Ω(x) = S(x)·λ(x) mod x^32 (error evaluator)
	var omega [rsParity]byte
	for i := range rsParity {
		var v byte
		for k := 0; k <= i && k <= l; k++ {
			v ^= r.mul(lambda[k], synd[i-k])
		}
		omega[i] = v
	}

	// Forney: e_j = X^(1-b0) · Ω(X⁻¹) / λ'(X⁻¹), b0 = 112
	for k := range nErr {
		j := errPos[k]
		xLog := (254 - j) % 255       // X = β^xLog
		xInvLog := (255 - xLog) % 255 // X⁻¹

		var om byte
		for i := rsParity - 1; i >= 0; i-- {
			om = r.mul(om, r.exp[xInvLog]) ^ omega[i]
		}
		// λ'(x): derivative keeps odd-power terms
		var lp byte
		for i := 1; i <= l; i += 2 {
			// term λ_i · x^(i-1) evaluated at X⁻¹
			lp ^= r.mul(lambda[i], r.exp[(xInvLog*(i-1))%255])
		}
		if lp == 0 {
			return -1
		}
		mag := r.div(om, lp)
		// multiply by X^(1-b0) = β^(xLog·(1-112) mod 255)
		expo := (xLog * (1 - rsFirstRoot)) % 255
		expo = (expo%255 + 255) % 255
		mag = r.mul(mag, r.exp[expo])
		cw[j] ^= mag
	}
	return nErr
}
