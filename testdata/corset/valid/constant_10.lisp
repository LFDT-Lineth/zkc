(defcolumns (X :i16) (Y :i16))
(defconst
  N 4
  TWO_N (* N N))

;; X == Y * 16
(defconstraint c1 () (== X (* Y TWO_N)))
