(defcolumns (x :i16) (y :i16 :padding 5))
(defconstraint c1 () (if (== 0 y) (== 0 x)))
