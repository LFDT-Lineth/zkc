;;error:3:41-49:multiple constant/padding values
(module m1)
(defcolumns (x :i16) (y :i16 :padding 0 :padding 0))

(defconstraint c1 () (if (== x y) (== 0 x)))
