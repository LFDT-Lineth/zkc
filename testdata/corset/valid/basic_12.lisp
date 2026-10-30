(module m1)
(defcolumns (x :i16) (y :i16 :padding 5))

(defconstraint c1 () (if (== 0 x) (== 5 y)))
