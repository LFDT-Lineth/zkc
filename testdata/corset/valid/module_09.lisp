(defcolumns (A :i16) (B :i16))
(module m1)
(defcolumns (A :i16) (B :i16))
(defconstraint eq () (== A B))
