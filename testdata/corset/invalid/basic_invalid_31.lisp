;;error:6:28-31:constant NEG is negative
(module m1)
(defconst NEG (- 0 1))
(defcolumns (x :i16))

(defconstraint c1 () (== x NEG))
