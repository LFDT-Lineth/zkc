(defconst
    ONE 1
    TWO 2
)

(defcolumns (X :i16) (Y :i64))
(defconstraint c1 () (== 0 (- X (* TWO Y))))
(defconstraint c2 () (== 0 (- (* TWO Y) X)))
(defconstraint c3 () (== 0 (- X Y Y)))
(defconstraint c5 () (== 0 (- (+ Y Y) X)))
(defconstraint c6 () (== 0 (- (* ONE X) Y Y)))
(defconstraint c7 () (== 0 (- (+ Y Y) (* ONE X))))
