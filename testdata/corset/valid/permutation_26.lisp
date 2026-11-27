;; Unchecked permutation, whose source column is wider than its target.
(defcolumns (X :i17) (Y :i16) (one :i1 :constant 1))
(defpermutation (test :unchecked) ((one X)) ((one Y)))
