(module m1)
(defcolumns (x :i16) (y :i16) (c :i16 :constant 1))

(defconstraint c1 () (if (== x c) (!= y c)))
