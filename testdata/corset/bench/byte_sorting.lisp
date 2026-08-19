;; A simple example which demonstrates how a sorting constraint can be
;; implemented on a column of bytes.

;; Input column
(defcolumns (X :i8@prove))

;; Generated column
(defcolumns (Delta :i8@prove))

;; Delta == X - X[i-1]
(defconstraint sort () (== Delta (- X (shift X -1))))
