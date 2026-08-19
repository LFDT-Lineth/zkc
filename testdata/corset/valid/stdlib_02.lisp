(defcolumns (A :i16) (B :i16) (C :i16))

(defconstraint c1 ()
  (if (!= A B) (== 0 C)))
