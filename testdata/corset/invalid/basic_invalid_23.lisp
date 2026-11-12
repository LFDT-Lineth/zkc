;;error:3:42-51:invalid constant column
(module m1)
(defcolumns (x :i16) (y :i16 :constant 1 :constant 1))

(defconstraint c1 () (if (== x y) (== 0 x)))
