(defcolumns (A :i2@prove) (B :i16) (C :i16))

;; (* (- A 1) (- A 2) (- A 3)) returns non-zero value if A is zero
(defconstraint c1 () (== 0 (* (* (- A 1) (- A 2) (- A 3)) B)))
(defconstraint c2 () (== 0 (* A C)))
