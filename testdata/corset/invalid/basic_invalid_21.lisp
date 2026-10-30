;;error:3:41-50:multiple constant/padding values
(module m1)
(defcolumns (x :i16) (y :i16 :padding 1 :constant 1))

(defconstraint c1 () (if (== x y) (== 0 x)))
