;;error:3:22-46:expected u16, found u17
(module m1)
(defcolumns (x :i16) (y :i16 :constant 65536))

(defconstraint c1 () (if (== x y) (== 0 x)))
