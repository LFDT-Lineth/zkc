(defcolumns (X :i16) (ST :i16))
(defconst ONE (+ -2 3))
(defconstraint c1 () (== 0 (* ST (shift X ONE))))
