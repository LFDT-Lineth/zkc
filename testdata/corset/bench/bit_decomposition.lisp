(defcolumns
  (NIBBLE :i4)
  (BIT_0 :i1@prove)
  (BIT_1 :i1@prove)
  (BIT_2 :i1@prove)
  (BIT_3 :i1@prove))

;; NIBBLE = 8*BIT_3 + 4*BIT_2 + 2*BIT_1 + BIT_0
(defconstraint decomp () (== NIBBLE (+ BIT_0 (* 2 BIT_1) (* 4 BIT_2) (* 8 BIT_3))))
