(defcolumns (X :i16) (Y :i16) (Z :i16))

(defconstraint c1 ()
  (if (if (== X Y) (!= 0 0) (== 0 0))
          (== 0 Z)))
