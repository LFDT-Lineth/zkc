(defcolumns (X :i16) (Y :i32))
;; Y == X*X
(defconstraint c1 () (== 0 (- Y (* X X))))
(defconstraint c2 () (== Y (* X X)))
