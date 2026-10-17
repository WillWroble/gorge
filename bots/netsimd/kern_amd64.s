//go:build amd64

#include "textflag.h"

// Every kernel is a whole-layer call: one entry, the full loop inside, so the
// no-inline cost of an assembly function is paid once per layer, not per
// lane. Scalars in [...]Scalar (kern_generic.go) spell the same arithmetic in
// the same order; a test holds the two bit-identical.

// func gemvTAVX512(wt, x, acc, y *float32, n, outPad int, relu bool)
//
// y[:outPad] = acc[:outPad] + Σ_{i<n} x[i]·wt[i*outPad : (i+1)*outPad]
// (then max(·,0) when relu). outPad is a multiple of 32. Columns are walked
// in blocks of 64 lanes (four zmm) with the i loop unrolled by two into even
// and odd accumulator sets — eight independent FMA chains, enough to keep
// both of Zen 5's 512-bit FMA pipes busy — then a 32-lane block for the
// remainder.
TEXT ·gemvTAVX512(SB), NOSPLIT, $0-49
	MOVQ wt+0(FP), AX
	MOVQ x+8(FP), BX
	MOVQ acc+16(FP), CX
	MOVQ y+24(FP), DX
	MOVQ n+32(FP), SI
	MOVQ outPad+40(FP), DI
	MOVBQZX relu+48(FP), R8
	SHLQ $2, DI             // row stride in bytes
	VPXORQ Z15, Z15, Z15    // zero, for relu
	XORQ R9, R9             // column offset in bytes

blk64:
	MOVQ DI, R10
	SUBQ R9, R10            // bytes of columns left
	CMPQ R10, $256
	JL   blk32
	VPXORQ Z0, Z0, Z0
	VPXORQ Z1, Z1, Z1
	VPXORQ Z2, Z2, Z2
	VPXORQ Z3, Z3, Z3
	VPXORQ Z6, Z6, Z6
	VPXORQ Z7, Z7, Z7
	VPXORQ Z8, Z8, Z8
	VPXORQ Z9, Z9, Z9
	LEAQ (AX)(R9*1), R11    // wt row i, this column block
	MOVQ BX, R12            // &x[i]
	MOVQ SI, R13
	SHRQ $1, R13            // pairs
	JZ   tail64

pair64:
	VBROADCASTSS (R12), Z4
	VBROADCASTSS 4(R12), Z5
	VFMADD231PS (R11), Z4, Z0
	VFMADD231PS 64(R11), Z4, Z1
	VFMADD231PS 128(R11), Z4, Z2
	VFMADD231PS 192(R11), Z4, Z3
	ADDQ DI, R11
	VFMADD231PS (R11), Z5, Z6
	VFMADD231PS 64(R11), Z5, Z7
	VFMADD231PS 128(R11), Z5, Z8
	VFMADD231PS 192(R11), Z5, Z9
	ADDQ DI, R11
	ADDQ $8, R12
	DECQ R13
	JNZ  pair64

tail64:
	TESTQ $1, SI
	JZ   sum64
	VBROADCASTSS (R12), Z4
	VFMADD231PS (R11), Z4, Z0
	VFMADD231PS 64(R11), Z4, Z1
	VFMADD231PS 128(R11), Z4, Z2
	VFMADD231PS 192(R11), Z4, Z3

sum64:
	VADDPS Z6, Z0, Z0       // even + odd
	VADDPS Z7, Z1, Z1
	VADDPS Z8, Z2, Z2
	VADDPS Z9, Z3, Z3
	VADDPS (CX)(R9*1), Z0, Z0   // acc + (even + odd)
	VADDPS 64(CX)(R9*1), Z1, Z1
	VADDPS 128(CX)(R9*1), Z2, Z2
	VADDPS 192(CX)(R9*1), Z3, Z3
	TESTQ R8, R8
	JZ   st64
	VMAXPS Z15, Z0, Z0
	VMAXPS Z15, Z1, Z1
	VMAXPS Z15, Z2, Z2
	VMAXPS Z15, Z3, Z3

st64:
	VMOVUPS Z0, (DX)(R9*1)
	VMOVUPS Z1, 64(DX)(R9*1)
	VMOVUPS Z2, 128(DX)(R9*1)
	VMOVUPS Z3, 192(DX)(R9*1)
	ADDQ $256, R9
	JMP  blk64

blk32:
	CMPQ R10, $128
	JL   done
	VPXORQ Z0, Z0, Z0
	VPXORQ Z1, Z1, Z1
	VPXORQ Z6, Z6, Z6
	VPXORQ Z7, Z7, Z7
	LEAQ (AX)(R9*1), R11
	MOVQ BX, R12
	MOVQ SI, R13
	SHRQ $1, R13
	JZ   tail32

pair32:
	VBROADCASTSS (R12), Z4
	VBROADCASTSS 4(R12), Z5
	VFMADD231PS (R11), Z4, Z0
	VFMADD231PS 64(R11), Z4, Z1
	ADDQ DI, R11
	VFMADD231PS (R11), Z5, Z6
	VFMADD231PS 64(R11), Z5, Z7
	ADDQ DI, R11
	ADDQ $8, R12
	DECQ R13
	JNZ  pair32

tail32:
	TESTQ $1, SI
	JZ   sum32
	VBROADCASTSS (R12), Z4
	VFMADD231PS (R11), Z4, Z0
	VFMADD231PS 64(R11), Z4, Z1

sum32:
	VADDPS Z6, Z0, Z0
	VADDPS Z7, Z1, Z1
	VADDPS (CX)(R9*1), Z0, Z0
	VADDPS 64(CX)(R9*1), Z1, Z1
	TESTQ R8, R8
	JZ   st32
	VMAXPS Z15, Z0, Z0
	VMAXPS Z15, Z1, Z1

st32:
	VMOVUPS Z0, (DX)(R9*1)
	VMOVUPS Z1, 64(DX)(R9*1)
	ADDQ $128, R9
	JMP  blk64

done:
	VZEROUPPER
	RET

// func bagAVX512(y, table *float32, feats *feat, n, rowLimit, width int)
//
// y[:width] += Σ feats[i].Value · table[row*width : (row+1)*width] over the
// features whose Row < rowLimit, in feature order, the running sum held in
// registers across the whole bag. width is 32 or 64. feat is 8 bytes: Row
// uint16 at 0, Value float32 at 4.
TEXT ·bagAVX512(SB), NOSPLIT, $0-48
	MOVQ y+0(FP), DX
	MOVQ table+8(FP), AX
	MOVQ feats+16(FP), SI
	MOVQ n+24(FP), CX
	MOVQ rowLimit+32(FP), DI
	MOVQ width+40(FP), R8
	SHLQ $2, R8             // row stride in bytes
	CMPQ R8, $256
	JE   bag64

	// width 32
	VMOVUPS (DX), Z0
	VMOVUPS 64(DX), Z1
	TESTQ CX, CX
	JZ   bst32
bl32:
	MOVWQZX (SI), R10
	CMPQ R10, DI
	JAE  bnext32
	IMULQ R8, R10
	VBROADCASTSS 4(SI), Z4
	VFMADD231PS (AX)(R10*1), Z4, Z0
	VFMADD231PS 64(AX)(R10*1), Z4, Z1
bnext32:
	ADDQ $8, SI
	DECQ CX
	JNZ  bl32
bst32:
	VMOVUPS Z0, (DX)
	VMOVUPS Z1, 64(DX)
	VZEROUPPER
	RET

bag64:
	VMOVUPS (DX), Z0
	VMOVUPS 64(DX), Z1
	VMOVUPS 128(DX), Z2
	VMOVUPS 192(DX), Z3
	TESTQ CX, CX
	JZ   bst64
bl64:
	MOVWQZX (SI), R10
	CMPQ R10, DI
	JAE  bnext64
	IMULQ R8, R10
	VBROADCASTSS 4(SI), Z4
	VFMADD231PS (AX)(R10*1), Z4, Z0
	VFMADD231PS 64(AX)(R10*1), Z4, Z1
	VFMADD231PS 128(AX)(R10*1), Z4, Z2
	VFMADD231PS 192(AX)(R10*1), Z4, Z3
bnext64:
	ADDQ $8, SI
	DECQ CX
	JNZ  bl64
bst64:
	VMOVUPS Z0, (DX)
	VMOVUPS Z1, 64(DX)
	VMOVUPS Z2, 128(DX)
	VMOVUPS Z3, 192(DX)
	VZEROUPPER
	RET

// hsum16 reduces Z4 (16 lanes) into X4 lane 0: 16→8→4, then
// (l0+l2)+(l1+l3). The scalars mirror this tree.
#define HSUM16 \
	VEXTRACTF64X4 $1, Z4, Y5 \
	VADDPS Y5, Y4, Y4 \
	VEXTRACTF128 $1, Y4, X5 \
	VADDPS X5, X4, X4 \
	VSHUFPS $0x4E, X4, X4, X5 \
	VADDPS X5, X4, X4 \
	VSHUFPS $0xB1, X4, X4, X5 \
	VADDSS X5, X4, X4

// func ln64AVX512(y, x, w, b *float32, eps float32)
//
// y = (x - mean)·rsqrt(var + eps)·w + b over 64 lanes, fp32. The mean and
// the variance reduce as ((q0+q1)+(q2+q3)) per lane, then HSUM16.
TEXT ·ln64AVX512(SB), NOSPLIT, $0-36
	MOVQ y+0(FP), DX
	MOVQ x+8(FP), AX
	MOVQ w+16(FP), BX
	MOVQ b+24(FP), CX
	VMOVUPS (AX), Z0
	VMOVUPS 64(AX), Z1
	VMOVUPS 128(AX), Z2
	VMOVUPS 192(AX), Z3
	// mean
	VADDPS Z1, Z0, Z4
	VADDPS Z3, Z2, Z5
	VADDPS Z5, Z4, Z4
	HSUM16
	MOVL $0x3c800000, R8    // 1/64
	MOVD R8, X6
	VMULSS X6, X4, X4
	VBROADCASTSS X4, Z4     // mean
	VSUBPS Z4, Z0, Z10
	VSUBPS Z4, Z1, Z11
	VSUBPS Z4, Z2, Z12
	VSUBPS Z4, Z3, Z13
	// variance
	VMULPS Z10, Z10, Z4
	VMULPS Z11, Z11, Z5
	VMULPS Z12, Z12, Z7
	VMULPS Z13, Z13, Z8
	VADDPS Z5, Z4, Z4
	VADDPS Z8, Z7, Z7
	VADDPS Z7, Z4, Z4
	HSUM16
	VMULSS X6, X4, X4       // q/64
	VADDSS eps+32(FP), X4, X4
	VSQRTSS X4, X4, X4
	MOVL $0x3f800000, R8    // 1.0
	MOVD R8, X6
	VDIVSS X4, X6, X4       // inv = 1/sqrt
	VBROADCASTSS X4, Z4
	VMULPS Z4, Z10, Z10
	VMULPS Z4, Z11, Z11
	VMULPS Z4, Z12, Z12
	VMULPS Z4, Z13, Z13
	VMULPS (BX), Z10, Z10
	VMULPS 64(BX), Z11, Z11
	VMULPS 128(BX), Z12, Z12
	VMULPS 192(BX), Z13, Z13
	VADDPS (CX), Z10, Z10
	VADDPS 64(CX), Z11, Z11
	VADDPS 128(CX), Z12, Z12
	VADDPS 192(CX), Z13, Z13
	VMOVUPS Z10, (DX)
	VMOVUPS Z11, 64(DX)
	VMOVUPS Z12, 128(DX)
	VMOVUPS Z13, 192(DX)
	VZEROUPPER
	RET

// func dot64AVX512(a, b *float32) float32
//
// One 16-lane chain over the four quarters, then HSUM16.
TEXT ·dot64AVX512(SB), NOSPLIT, $0-20
	MOVQ a+0(FP), AX
	MOVQ b+8(FP), BX
	VMOVUPS (AX), Z4
	VMULPS (BX), Z4, Z4
	VMOVUPS 64(AX), Z0
	VFMADD231PS 64(BX), Z0, Z4
	VMOVUPS 128(AX), Z1
	VFMADD231PS 128(BX), Z1, Z4
	VMOVUPS 192(AX), Z2
	VFMADD231PS 192(BX), Z2, Z4
	HSUM16
	VMOVSS X4, ret+16(FP)
	VZEROUPPER
	RET

// func addv64AVX512(y, x *float32)
TEXT ·addv64AVX512(SB), NOSPLIT, $0-16
	MOVQ y+0(FP), DX
	MOVQ x+8(FP), AX
	VMOVUPS (DX), Z0
	VMOVUPS 64(DX), Z1
	VMOVUPS 128(DX), Z2
	VMOVUPS 192(DX), Z3
	VADDPS (AX), Z0, Z0
	VADDPS 64(AX), Z1, Z1
	VADDPS 128(AX), Z2, Z2
	VADDPS 192(AX), Z3, Z3
	VMOVUPS Z0, (DX)
	VMOVUPS Z1, 64(DX)
	VMOVUPS Z2, 128(DX)
	VMOVUPS Z3, 192(DX)
	VZEROUPPER
	RET

// func poolAccAVX512(sum, mx, c *float32)
//
// sum[:32] += 0.25·c; mx[:32] = max(mx, c).
TEXT ·poolAccAVX512(SB), NOSPLIT, $0-24
	MOVQ sum+0(FP), DX
	MOVQ mx+8(FP), BX
	MOVQ c+16(FP), AX
	MOVL $0x3e800000, R8    // 0.25
	MOVD R8, X4
	VBROADCASTSS X4, Z4
	VMOVUPS (AX), Z0
	VMOVUPS 64(AX), Z1
	VMOVUPS (DX), Z2
	VMOVUPS 64(DX), Z3
	VFMADD231PS Z0, Z4, Z2
	VFMADD231PS Z1, Z4, Z3
	VMOVUPS Z2, (DX)
	VMOVUPS Z3, 64(DX)
	VMOVUPS (BX), Z2
	VMOVUPS 64(BX), Z3
	VMAXPS Z0, Z2, Z2
	VMAXPS Z1, Z3, Z3
	VMOVUPS Z2, (BX)
	VMOVUPS Z3, 64(BX)
	VZEROUPPER
	RET
