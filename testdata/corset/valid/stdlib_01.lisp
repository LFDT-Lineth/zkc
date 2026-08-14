(defcolumns (A :i32) (B :i16) (C :i32))

(defconstraint c1 () (if (!= A B) (== 0 C)))
